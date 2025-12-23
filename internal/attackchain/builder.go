package attackchain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cyberstrike-ai/internal/agent"
	"cyberstrike-ai/internal/config"
	"cyberstrike-ai/internal/database"
	"cyberstrike-ai/internal/openai"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Builder 攻击链构建器
type Builder struct {
	db           *database.DB
	logger       *zap.Logger
	openAIClient *openai.Client
	openAIConfig *config.OpenAIConfig
	tokenCounter agent.TokenCounter
	maxTokens    int // 最大tokens限制，默认100000
}

// Node 攻击链节点（使用database包的类型）
type Node = database.AttackChainNode

// Edge 攻击链边（使用database包的类型）
type Edge = database.AttackChainEdge

// Chain 完整的攻击链
type Chain struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// NewBuilder 创建新的攻击链构建器
func NewBuilder(db *database.DB, openAIConfig *config.OpenAIConfig, logger *zap.Logger) *Builder {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	httpClient := &http.Client{Timeout: 5 * time.Minute, Transport: transport}

	// 优先使用配置文件中的统一 Token 上限（config.yaml -> openai.max_total_tokens）
	maxTokens := 0
	if openAIConfig != nil && openAIConfig.MaxTotalTokens > 0 {
		maxTokens = openAIConfig.MaxTotalTokens
	} else if openAIConfig != nil {
		// 如果未显式配置 max_total_tokens，则根据模型设置一个合理的默认值
		model := strings.ToLower(openAIConfig.Model)
		if strings.Contains(model, "gpt-4") {
			maxTokens = 128000 // gpt-4通常支持128k
		} else if strings.Contains(model, "gpt-3.5") {
			maxTokens = 16000 // gpt-3.5-turbo通常支持16k
		} else if strings.Contains(model, "deepseek") {
			maxTokens = 131072 // deepseek-chat通常支持131k
		} else {
			maxTokens = 100000 // 兜底默认值
		}
	} else {
		// 没有 OpenAI 配置时使用兜底值，避免为 0
		maxTokens = 100000
	}

	return &Builder{
		db:           db,
		logger:       logger,
		openAIClient: openai.NewClient(openAIConfig, httpClient, logger),
		openAIConfig: openAIConfig,
		tokenCounter: agent.NewTikTokenCounter(),
		maxTokens:    maxTokens,
	}
}

// BuildChainFromConversation 从对话构建攻击链（简化版本：用户输入+最后一轮ReAct输入+大模型输出）
func (b *Builder) BuildChainFromConversation(ctx context.Context, conversationID string) (*Chain, error) {
	b.logger.Info("开始构建攻击链（简化版本）", zap.String("conversationId", conversationID))

	// 0. 首先检查是否有实际的工具执行记录
	messages, err := b.db.GetMessages(conversationID)
	if err != nil {
		return nil, fmt.Errorf("获取对话消息失败: %w", err)
	}

	if len(messages) == 0 {
		b.logger.Info("对话中没有数据", zap.String("conversationId", conversationID))
		return &Chain{Nodes: []Node{}, Edges: []Edge{}}, nil
	}

	// 检查是否有实际的工具执行（通过检查assistant消息的mcp_execution_ids）
	hasToolExecutions := false
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "assistant") {
			if len(messages[i].MCPExecutionIDs) > 0 {
				hasToolExecutions = true
				break
			}
		}
	}

	// 检查任务是否被取消（通过检查最后一条assistant消息内容或process_details）
	taskCancelled := false
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "assistant") {
			content := strings.ToLower(messages[i].Content)
			if strings.Contains(content, "取消") || strings.Contains(content, "cancelled") {
				taskCancelled = true
			}
			break
		}
	}

	// 如果任务被取消且没有实际工具执行，返回空攻击链
	if taskCancelled && !hasToolExecutions {
		b.logger.Info("任务已取消且没有实际工具执行，返回空攻击链",
			zap.String("conversationId", conversationID),
			zap.Bool("taskCancelled", taskCancelled),
			zap.Bool("hasToolExecutions", hasToolExecutions))
		return &Chain{Nodes: []Node{}, Edges: []Edge{}}, nil
	}

	// 如果没有实际工具执行，也返回空攻击链（避免AI编造）
	if !hasToolExecutions {
		b.logger.Info("没有实际工具执行记录，返回空攻击链",
			zap.String("conversationId", conversationID))
		return &Chain{Nodes: []Node{}, Edges: []Edge{}}, nil
	}

	// 1. 优先尝试从数据库获取保存的最后一轮ReAct输入和输出
	reactInputJSON, modelOutput, err := b.db.GetReActData(conversationID)
	if err != nil {
		b.logger.Warn("获取保存的ReAct数据失败，将使用消息历史构建", zap.Error(err))
		// 继续使用原来的逻辑
		reactInputJSON = ""
		modelOutput = ""
	}

	// var userInput string
	var reactInputFinal string
	var dataSource string // 记录数据来源

	// 如果成功获取到保存的ReAct数据，直接使用
	if reactInputJSON != "" && modelOutput != "" {
		// 计算 ReAct 输入的哈希值，用于追踪
		hash := sha256.Sum256([]byte(reactInputJSON))
		reactInputHash := hex.EncodeToString(hash[:])[:16] // 使用前16字符作为短标识

		// 统计消息数量
		var messageCount int
		var tempMessages []interface{}
		if json.Unmarshal([]byte(reactInputJSON), &tempMessages) == nil {
			messageCount = len(tempMessages)
		}

		dataSource = "database_last_react_input"
		b.logger.Info("使用保存的ReAct数据构建攻击链",
			zap.String("conversationId", conversationID),
			zap.String("dataSource", dataSource),
			zap.Int("reactInputSize", len(reactInputJSON)),
			zap.Int("messageCount", messageCount),
			zap.String("reactInputHash", reactInputHash),
			zap.Int("modelOutputSize", len(modelOutput)))

		// 从保存的ReAct输入（JSON格式）中提取用户输入
		// userInput = b.extractUserInputFromReActInput(reactInputJSON)

		// 将JSON格式的messages转换为可读格式
		reactInputFinal = b.formatReActInputFromJSON(reactInputJSON)
	} else {
		// 2. 如果没有保存的ReAct数据，从对话消息构建
		dataSource = "messages_table"
		b.logger.Info("从消息历史构建ReAct数据",
			zap.String("conversationId", conversationID),
			zap.String("dataSource", dataSource),
			zap.Int("messageCount", len(messages)))

		// 提取用户输入（最后一条user消息）
		for i := len(messages) - 1; i >= 0; i-- {
			if strings.EqualFold(messages[i].Role, "user") {
				// userInput = messages[i].Content
				break
			}
		}

		// 提取最后一轮ReAct的输入（历史消息+当前用户输入）
		reactInputFinal = b.buildReActInput(messages)

		// 提取大模型最后的输出（最后一条assistant消息）
		for i := len(messages) - 1; i >= 0; i-- {
			if strings.EqualFold(messages[i].Role, "assistant") {
				modelOutput = messages[i].Content
				break
			}
		}
	}

	// 3. 构建简化的prompt，一次性传递给大模型
	prompt := b.buildSimplePrompt(reactInputFinal, modelOutput)
	// fmt.Println(prompt)
	// 6. 调用AI生成攻击链（一次性，不做任何处理）
	chainJSON, err := b.callAIForChainGeneration(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("AI生成失败: %w", err)
	}

	// 7. 解析JSON并生成节点/边ID（前端需要有效的ID）
	chainData, err := b.parseChainJSON(chainJSON)
	if err != nil {
		// 如果解析失败，返回空链，让前端处理错误
		b.logger.Warn("解析攻击链JSON失败", zap.Error(err), zap.String("raw_json", chainJSON))
		return &Chain{
			Nodes: []Node{},
			Edges: []Edge{},
		}, nil
	}

	b.logger.Info("攻击链构建完成",
		zap.String("conversationId", conversationID),
		zap.String("dataSource", dataSource),
		zap.Int("nodes", len(chainData.Nodes)),
		zap.Int("edges", len(chainData.Edges)))

	// 保存到数据库（供后续加载使用）
	if err := b.saveChain(conversationID, chainData.Nodes, chainData.Edges); err != nil {
		b.logger.Warn("保存攻击链到数据库失败", zap.Error(err))
		// 即使保存失败，也返回数据给前端
	}

	// 直接返回，不做任何处理和校验
	return chainData, nil
}

// buildReActInput 构建最后一轮ReAct的输入（历史消息+当前用户输入）
func (b *Builder) buildReActInput(messages []database.Message) string {
	var builder strings.Builder
	for _, msg := range messages {
		builder.WriteString(fmt.Sprintf("[%s]: %s\n\n", msg.Role, msg.Content))
	}
	return builder.String()
}

// extractUserInputFromReActInput 从保存的ReAct输入（JSON格式的messages数组）中提取最后一条用户输入
// func (b *Builder) extractUserInputFromReActInput(reactInputJSON string) string {
// 	// reactInputJSON是JSON格式的ChatMessage数组，需要解析
// 	var messages []map[string]interface{}
// 	if err := json.Unmarshal([]byte(reactInputJSON), &messages); err != nil {
// 		b.logger.Warn("解析ReAct输入JSON失败", zap.Error(err))
// 		return ""
// 	}

// 	// 从后往前查找最后一条user消息
// 	for i := len(messages) - 1; i >= 0; i-- {
// 		if role, ok := messages[i]["role"].(string); ok && strings.EqualFold(role, "user") {
// 			if content, ok := messages[i]["content"].(string); ok {
// 				return content
// 			}
// 		}
// 	}

// 	return ""
// }

// formatReActInputFromJSON 将JSON格式的messages数组转换为可读的字符串格式
func (b *Builder) formatReActInputFromJSON(reactInputJSON string) string {
	var messages []map[string]interface{}
	if err := json.Unmarshal([]byte(reactInputJSON), &messages); err != nil {
		b.logger.Warn("解析ReAct输入JSON失败", zap.Error(err))
		return reactInputJSON // 如果解析失败，返回原始JSON
	}

	var builder strings.Builder
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)

		// 处理assistant消息：提取tool_calls信息
		if role == "assistant" {
			if toolCalls, ok := msg["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
				// 如果有文本内容，先显示
				if content != "" {
					builder.WriteString(fmt.Sprintf("[%s]: %s\n", role, content))
				}
				// 详细显示每个工具调用
				builder.WriteString(fmt.Sprintf("[%s] 工具调用 (%d个):\n", role, len(toolCalls)))
				for i, toolCall := range toolCalls {
					if tc, ok := toolCall.(map[string]interface{}); ok {
						toolCallID, _ := tc["id"].(string)
						if funcData, ok := tc["function"].(map[string]interface{}); ok {
							toolName, _ := funcData["name"].(string)
							arguments, _ := funcData["arguments"].(string)
							builder.WriteString(fmt.Sprintf("  [工具调用 %d]\n", i+1))
							builder.WriteString(fmt.Sprintf("    ID: %s\n", toolCallID))
							builder.WriteString(fmt.Sprintf("    工具名称: %s\n", toolName))
							builder.WriteString(fmt.Sprintf("    参数: %s\n", arguments))
						}
					}
				}
				builder.WriteString("\n")
				continue
			}
		}

		// 处理tool消息：显示tool_call_id和完整内容
		if role == "tool" {
			toolCallID, _ := msg["tool_call_id"].(string)
			if toolCallID != "" {
				builder.WriteString(fmt.Sprintf("[%s] (tool_call_id: %s):\n%s\n\n", role, toolCallID, content))
			} else {
				builder.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, content))
			}
			continue
		}

		// 其他消息类型（system, user等）正常显示
		builder.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, content))
	}

	return builder.String()
}

// buildSimplePrompt 构建简化的prompt
func (b *Builder) buildSimplePrompt(reactInput, modelOutput string) string {
	return fmt.Sprintf(`你是一个专业的安全测试分析师。请根据以下信息生成攻击链图。

## ⚠️ 重要原则 - 严禁杜撰

**严格禁止编造或推测任何内容！** 你必须：
1. **只使用实际发生的信息**：仅基于ReAct输入中实际执行的工具调用和实际返回的结果
2. **不要推测**：如果没有实际执行工具或发现漏洞，不要编造
3. **不要假设**：不能仅根据URL、目标名称等推断漏洞类型
4. **基于事实**：每个节点和边都必须有实际依据，来自工具执行结果或模型的实际输出

如果ReAct输入中没有实际的工具执行记录，或者模型输出中明确表示任务未完成/被取消，必须返回空的攻击链（空的nodes和edges数组）。



## 最后一轮ReAct的输入（历史对话上下文）
%s

## 大模型最后的输出
%s

## 任务要求

请根据上述信息，**仅基于实际执行的数据**生成一个清晰的攻击链图。攻击链应该包含：
1. **target（目标）**：从用户输入中提取的实际测试目标（必须是用户明确提供的）
2. **action（行动）**：从ReAct输入中提取的**实际执行的**工具调用和测试步骤（必须有tool_calls证据）
3. **vulnerability（漏洞）**：从模型输出中提取的**实际发现的**漏洞（必须在输出中明确提及，不能推测）

**关键检查点：**
- 如果ReAct输入中没有tool_calls，说明没有实际执行工具 → 只能生成target节点
- 如果模型输出中没有明确提到发现的漏洞，不要编造vulnerability节点
- 如果任务被取消或未完成，返回空攻击链

## 输出格式

请以JSON格式返回攻击链，格式如下：
{
   "nodes": [
     {
       "id": "node_1",
       "type": "target|action|vulnerability",
       "label": "节点标签",
       "risk_score": 0-100,
       "metadata": {
         "target": "目标（target节点）",
         "tool_name": "工具名称（action节点，必须是实际调用的工具）",
         "description": "描述（vulnerability节点，必须是实际发现的漏洞）"
       }
     }
   ],
   "edges": [
     {
       "source": "node_1",
       "target": "node_2",
       "type": "leads_to|discovers|enables",
       "weight": 1-5
     }
   ]
}

**再次强调：如果没有实际数据，返回空的nodes和edges数组。严禁杜撰！**

只返回JSON，不要包含其他解释文字。`, reactInput, modelOutput)
}

// saveChain 保存攻击链到数据库（简化版本，移除tool_execution_id）
func (b *Builder) saveChain(conversationID string, nodes []Node, edges []Edge) error {
	// 先删除旧的攻击链数据
	if err := b.db.DeleteAttackChain(conversationID); err != nil {
		b.logger.Warn("删除旧攻击链失败", zap.Error(err))
	}

	// 保存节点（不保存tool_execution_id）
	for _, node := range nodes {
		metadataJSON, _ := json.Marshal(node.Metadata)
		if err := b.db.SaveAttackChainNode(conversationID, node.ID, node.Type, node.Label, "", string(metadataJSON), node.RiskScore); err != nil {
			b.logger.Warn("保存攻击链节点失败", zap.String("nodeId", node.ID), zap.Error(err))
		}
	}

	// 保存边
	for _, edge := range edges {
		if err := b.db.SaveAttackChainEdge(conversationID, edge.ID, edge.Source, edge.Target, edge.Type, edge.Weight); err != nil {
			b.logger.Warn("保存攻击链边失败", zap.String("edgeId", edge.ID), zap.Error(err))
		}
	}

	return nil
}

// LoadChainFromDatabase 从数据库加载攻击链
func (b *Builder) LoadChainFromDatabase(conversationID string) (*Chain, error) {
	nodes, err := b.db.LoadAttackChainNodes(conversationID)
	if err != nil {
		return nil, fmt.Errorf("加载攻击链节点失败: %w", err)
	}

	edges, err := b.db.LoadAttackChainEdges(conversationID)
	if err != nil {
		return nil, fmt.Errorf("加载攻击链边失败: %w", err)
	}

	return &Chain{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// callAIForChainGeneration 调用AI生成攻击链
func (b *Builder) callAIForChainGeneration(ctx context.Context, prompt string) (string, error) {
	requestBody := map[string]interface{}{
		"model": b.openAIConfig.Model,
		"messages": []map[string]interface{}{
			{
				"role":    "system",
				"content": "你是一个专业的安全测试分析师，擅长构建攻击链图。请严格按照JSON格式返回攻击链数据。",
			},
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.3,
		"max_tokens":  8000,
	}

	var apiResponse struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if b.openAIClient == nil {
		return "", fmt.Errorf("OpenAI客户端未初始化")
	}
	if err := b.openAIClient.ChatCompletion(ctx, requestBody, &apiResponse); err != nil {
		var apiErr *openai.APIError
		if errors.As(err, &apiErr) {
			bodyStr := strings.ToLower(apiErr.Body)
			if strings.Contains(bodyStr, "context") || strings.Contains(bodyStr, "length") || strings.Contains(bodyStr, "too long") {
				return "", fmt.Errorf("context length exceeded")
			}
		} else if strings.Contains(strings.ToLower(err.Error()), "context") || strings.Contains(strings.ToLower(err.Error()), "length") {
			return "", fmt.Errorf("context length exceeded")
		}
		return "", fmt.Errorf("请求失败: %w", err)
	}

	if len(apiResponse.Choices) == 0 {
		return "", fmt.Errorf("API未返回有效响应")
	}

	content := strings.TrimSpace(apiResponse.Choices[0].Message.Content)
	// 尝试提取JSON（可能包含markdown代码块）
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	return content, nil
}

// ChainJSON 攻击链JSON结构
type ChainJSON struct {
	Nodes []struct {
		ID              string                 `json:"id"`
		Type            string                 `json:"type"`
		Label           string                 `json:"label"`
		RiskScore       int                    `json:"risk_score"`
		ToolExecutionID string                 `json:"tool_execution_id,omitempty"`
		Metadata        map[string]interface{} `json:"metadata"`
	} `json:"nodes"`
	Edges []struct {
		Source string `json:"source"`
		Target string `json:"target"`
		Type   string `json:"type"`
		Weight int    `json:"weight"`
	} `json:"edges"`
}

// parseChainJSON 解析攻击链JSON
func (b *Builder) parseChainJSON(chainJSON string) (*Chain, error) {
	var chainData ChainJSON
	if err := json.Unmarshal([]byte(chainJSON), &chainData); err != nil {
		return nil, fmt.Errorf("解析JSON失败: %w", err)
	}

	// 创建节点ID映射（AI返回的ID -> 新的UUID）
	nodeIDMap := make(map[string]string)

	// 转换为Chain结构
	nodes := make([]Node, 0, len(chainData.Nodes))
	for _, n := range chainData.Nodes {
		// 生成新的UUID节点ID
		newNodeID := fmt.Sprintf("node_%s", uuid.New().String())
		nodeIDMap[n.ID] = newNodeID

		node := Node{
			ID:        newNodeID,
			Type:      n.Type,
			Label:     n.Label,
			RiskScore: n.RiskScore,
			Metadata:  n.Metadata,
		}
		if node.Metadata == nil {
			node.Metadata = make(map[string]interface{})
		}
		nodes = append(nodes, node)
	}

	// 转换边
	edges := make([]Edge, 0, len(chainData.Edges))
	for _, e := range chainData.Edges {
		sourceID, ok := nodeIDMap[e.Source]
		if !ok {
			continue
		}
		targetID, ok := nodeIDMap[e.Target]
		if !ok {
			continue
		}

		// 生成边的ID（前端需要）
		edgeID := fmt.Sprintf("edge_%s", uuid.New().String())

		edges = append(edges, Edge{
			ID:     edgeID,
			Source: sourceID,
			Target: targetID,
			Type:   e.Type,
			Weight: e.Weight,
		})
	}

	return &Chain{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// 以下所有方法已不再使用，已删除以简化代码
