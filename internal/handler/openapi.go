package handler

import (
	"net/http"
	"time"

	"cyberstrike-ai/internal/database"
	"cyberstrike-ai/internal/storage"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// OpenAPIHandler OpenAPI处理器
type OpenAPIHandler struct {
	db              *database.DB
	logger          *zap.Logger
	resultStorage   storage.ResultStorage
	conversationHdlr *ConversationHandler
	agentHdlr       *AgentHandler
}

// NewOpenAPIHandler 创建新的OpenAPI处理器
func NewOpenAPIHandler(db *database.DB, logger *zap.Logger, resultStorage storage.ResultStorage, conversationHdlr *ConversationHandler, agentHdlr *AgentHandler) *OpenAPIHandler {
	return &OpenAPIHandler{
		db:              db,
		logger:          logger,
		resultStorage:   resultStorage,
		conversationHdlr: conversationHdlr,
		agentHdlr:       agentHdlr,
	}
}

// GetOpenAPISpec 获取OpenAPI规范
func (h *OpenAPIHandler) GetOpenAPISpec(c *gin.Context) {
	host := c.Request.Host
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}

	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "CyberStrikeAI API",
			"description": "AI驱动的自动化安全测试平台API文档",
			"version":     "1.0.0",
			"contact": map[string]interface{}{
				"name": "CyberStrikeAI",
			},
		},
		"servers": []map[string]interface{}{
			{
				"url":         scheme + "://" + host,
				"description": "当前服务器",
			},
		},
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{
				"bearerAuth": map[string]interface{}{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
					"description":  "使用Bearer Token进行认证。Token通过 /api/auth/login 接口获取。",
				},
			},
			"schemas": map[string]interface{}{
				"CreateConversationRequest": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"title": map[string]interface{}{
							"type":        "string",
							"description": "对话标题",
							"example":     "Web应用安全测试",
						},
					},
				},
				"Conversation": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "对话ID",
							"example":     "550e8400-e29b-41d4-a716-446655440000",
						},
						"title": map[string]interface{}{
							"type":        "string",
							"description": "对话标题",
							"example":     "Web应用安全测试",
						},
						"createdAt": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "创建时间",
						},
						"updatedAt": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "更新时间",
						},
					},
				},
				"ConversationDetail": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "对话ID",
						},
						"title": map[string]interface{}{
							"type":        "string",
							"description": "对话标题",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "对话状态：active（进行中）、completed（已完成）、failed（失败）",
							"enum":        []string{"active", "completed", "failed"},
						},
						"createdAt": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "创建时间",
						},
						"updatedAt": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "更新时间",
						},
						"messages": map[string]interface{}{
							"type":        "array",
							"description": "消息列表",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/Message",
							},
						},
						"messageCount": map[string]interface{}{
							"type":        "integer",
							"description": "消息数量",
						},
					},
				},
				"Message": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "消息ID",
						},
						"conversationId": map[string]interface{}{
							"type":        "string",
							"description": "对话ID",
						},
						"role": map[string]interface{}{
							"type":        "string",
							"description": "消息角色：user（用户）、assistant（助手）",
							"enum":        []string{"user", "assistant"},
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "消息内容",
						},
						"createdAt": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "创建时间",
						},
					},
				},
				"ConversationResults": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"conversationId": map[string]interface{}{
							"type":        "string",
							"description": "对话ID",
						},
						"messages": map[string]interface{}{
							"type":        "array",
							"description": "消息列表",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/Message",
							},
						},
						"vulnerabilities": map[string]interface{}{
							"type":        "array",
							"description": "发现的漏洞列表",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/Vulnerability",
							},
						},
						"executionResults": map[string]interface{}{
							"type":        "array",
							"description": "执行结果列表",
							"items": map[string]interface{}{
								"$ref": "#/components/schemas/ExecutionResult",
							},
						},
					},
				},
				"Vulnerability": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "漏洞ID",
						},
						"title": map[string]interface{}{
							"type":        "string",
							"description": "漏洞标题",
						},
						"description": map[string]interface{}{
							"type":        "string",
							"description": "漏洞描述",
						},
						"severity": map[string]interface{}{
							"type":        "string",
							"description": "严重程度",
							"enum":        []string{"critical", "high", "medium", "low", "info"},
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "状态",
							"enum":        []string{"open", "closed", "fixed"},
						},
						"target": map[string]interface{}{
							"type":        "string",
							"description": "受影响的目标",
						},
					},
				},
				"ExecutionResult": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"id": map[string]interface{}{
							"type":        "string",
							"description": "执行ID",
						},
						"toolName": map[string]interface{}{
							"type":        "string",
							"description": "工具名称",
						},
						"status": map[string]interface{}{
							"type":        "string",
							"description": "执行状态",
							"enum":        []string{"success", "failed", "running"},
						},
						"result": map[string]interface{}{
							"type":        "string",
							"description": "执行结果",
						},
						"createdAt": map[string]interface{}{
							"type":        "string",
							"format":      "date-time",
							"description": "创建时间",
						},
					},
				},
				"Error": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"error": map[string]interface{}{
							"type":        "string",
							"description": "错误信息",
						},
					},
				},
			},
		},
		"security": []map[string]interface{}{
			{
				"bearerAuth": []string{},
			},
		},
		"paths": map[string]interface{}{
			"/api/conversations": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"对话管理"},
					"summary":     "创建对话",
					"description": "创建一个新的安全测试对话。\n\n**重要说明**：\n- ✅ 创建的对话会**立即保存到数据库**\n- ✅ 前端页面会**自动刷新**显示新对话\n- ✅ 与前端创建的对话**完全一致**\n\n**创建对话的两种方式**：\n\n**方式1（推荐）：** 直接使用 `/api/agent-loop` 发送消息，**不提供** `conversationId` 参数，系统会自动创建新对话并发送消息。这是最简单的方式，一步完成创建和发送。\n\n**方式2：** 先调用此端点创建空对话，然后使用返回的 `conversationId` 调用 `/api/agent-loop` 发送消息。适用于需要先创建对话，稍后再发送消息的场景。\n\n**示例**：\n```json\n{\n  \"title\": \"Web应用安全测试\"\n}\n```",
					"operationId": "createConversation",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"$ref": "#/components/schemas/CreateConversationRequest",
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "对话创建成功",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/Conversation",
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "请求参数错误",
						},
						"401": map[string]interface{}{
							"description": "未授权，需要有效的Token",
						},
						"500": map[string]interface{}{
							"description": "服务器内部错误",
						},
					},
				},
				"get": map[string]interface{}{
					"tags":        []string{"对话管理"},
					"summary":     "列出对话",
					"description": "获取对话列表，支持分页和搜索",
					"operationId": "listConversations",
					"parameters": []map[string]interface{}{
						{
							"name":        "limit",
							"in":          "query",
							"required":    false,
							"description": "返回数量限制",
							"schema": map[string]interface{}{
								"type":    "integer",
								"default": 50,
								"minimum": 1,
								"maximum": 100,
							},
						},
						{
							"name":        "offset",
							"in":          "query",
							"required":    false,
							"description": "偏移量",
							"schema": map[string]interface{}{
								"type":    "integer",
								"default": 0,
								"minimum": 0,
							},
						},
						{
							"name":        "search",
							"in":          "query",
							"required":    false,
							"description": "搜索关键词",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "获取成功",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "array",
										"items": map[string]interface{}{
											"$ref": "#/components/schemas/Conversation",
										},
									},
								},
							},
						},
						"401": map[string]interface{}{
							"description": "未授权，需要有效的Token",
						},
					},
				},
			},
			"/api/conversations/{id}": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"对话管理"},
					"summary":     "查看对话详情",
					"description": "获取指定对话的详细信息，包括对话信息和消息列表",
					"operationId": "getConversation",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "对话ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "获取成功",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ConversationDetail",
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "对话不存在",
						},
						"401": map[string]interface{}{
							"description": "未授权，需要有效的Token",
						},
					},
				},
				"delete": map[string]interface{}{
					"tags":        []string{"对话管理"},
					"summary":     "删除对话",
					"description": "删除指定的对话及其所有相关数据（消息、漏洞等）。**此操作不可恢复**。",
					"operationId": "deleteConversation",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "对话ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "删除成功",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"message": map[string]interface{}{
												"type":        "string",
												"description": "成功消息",
												"example":     "删除成功",
											},
										},
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "对话不存在",
						},
						"401": map[string]interface{}{
							"description": "未授权，需要有效的Token",
						},
						"500": map[string]interface{}{
							"description": "服务器内部错误",
						},
					},
				},
			},
			"/api/conversations/{id}/results": map[string]interface{}{
				"get": map[string]interface{}{
					"tags":        []string{"结果查询"},
					"summary":     "获取对话结果",
					"description": "获取指定对话的执行结果，包括消息、漏洞信息和执行结果",
					"operationId": "getConversationResults",
					"parameters": []map[string]interface{}{
						{
							"name":        "id",
							"in":          "path",
							"required":    true,
							"description": "对话ID",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "获取成功",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"$ref": "#/components/schemas/ConversationResults",
									},
								},
							},
						},
						"404": map[string]interface{}{
							"description": "对话不存在或结果不存在",
						},
						"401": map[string]interface{}{
							"description": "未授权，需要有效的Token",
						},
					},
				},
			},
			"/api/agent-loop": map[string]interface{}{
				"post": map[string]interface{}{
					"tags":        []string{"对话交互"},
					"summary":     "发送消息并获取AI回复（核心端点）",
					"description": "向AI发送消息并获取回复。**这是与AI交互的核心端点**，与前端聊天功能完全一致。\n\n**重要说明**：\n- ✅ 通过此API创建/发送的消息会**立即保存到数据库**\n- ✅ 前端页面会**自动刷新**显示新创建的对话和消息\n- ✅ 所有操作都有**完整的交互痕迹**，就像在前端操作一样\n- ✅ 支持角色配置，可以指定使用哪个测试角色\n\n**推荐使用流程**：\n\n1. **先创建对话**：调用 `POST /api/conversations` 创建新对话，获取 `conversationId`\n2. **再发送消息**：使用返回的 `conversationId` 调用此端点发送消息\n\n**使用示例**：\n\n**步骤1 - 创建对话：**\n```json\nPOST /api/conversations\n{\n  \"title\": \"Web应用安全测试\"\n}\n```\n\n**步骤2 - 发送消息：**\n```json\nPOST /api/agent-loop\n{\n  \"conversationId\": \"返回的对话ID\",\n  \"message\": \"扫描 http://example.com 的SQL注入漏洞\",\n  \"role\": \"渗透测试\"\n}\n```\n\n**其他方式**：\n\n如果不提供 `conversationId`，系统会自动创建新对话并发送消息。但**推荐先创建对话**，这样可以更好地管理对话列表。\n\n**响应**：返回AI的回复、对话ID和MCP执行ID列表。前端会自动刷新显示新消息。",
					"operationId": "sendMessage",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"message": map[string]interface{}{
											"type":        "string",
											"description": "要发送的消息（必需）",
											"example":     "扫描 http://example.com 的SQL注入漏洞",
										},
										"conversationId": map[string]interface{}{
											"type":        "string",
											"description": "对话ID（可选）。\n- **不提供**：自动创建新对话并发送消息（推荐）\n- **提供**：消息会添加到指定对话中（对话必须存在）",
											"example":     "550e8400-e29b-41d4-a716-446655440000",
										},
										"role": map[string]interface{}{
											"type":        "string",
											"description": "角色名称（可选），如：默认、渗透测试、Web应用扫描等",
											"example":     "默认",
										},
									},
									"required": []string{"message"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "消息发送成功，返回AI回复",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type": "object",
										"properties": map[string]interface{}{
											"response": map[string]interface{}{
												"type":        "string",
												"description": "AI的回复内容",
											},
											"conversationId": map[string]interface{}{
												"type":        "string",
												"description": "对话ID",
											},
											"mcpExecutionIds": map[string]interface{}{
												"type":        "array",
												"description": "MCP执行ID列表",
												"items": map[string]interface{}{
													"type": "string",
												},
											},
											"time": map[string]interface{}{
												"type":        "string",
												"format":      "date-time",
												"description": "响应时间",
											},
										},
									},
								},
							},
						},
						"400": map[string]interface{}{
							"description": "请求参数错误",
						},
						"401": map[string]interface{}{
							"description": "未授权，需要有效的Token",
						},
						"500": map[string]interface{}{
							"description": "服务器内部错误",
						},
					},
				},
			},
		},
	}

	c.JSON(http.StatusOK, spec)
}


// GetConversationResults 获取对话结果（OpenAPI端点）
// 注意：创建对话和获取对话详情直接使用标准的 /api/conversations 端点
// 这个端点只是为了提供结果聚合功能
func (h *OpenAPIHandler) GetConversationResults(c *gin.Context) {
	conversationID := c.Param("id")

	// 验证对话是否存在
	conv, err := h.db.GetConversation(conversationID)
	if err != nil {
		h.logger.Error("获取对话失败", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "对话不存在"})
		return
	}

	// 获取消息列表
	messages, err := h.db.GetMessages(conversationID)
	if err != nil {
		h.logger.Error("获取消息失败", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取漏洞列表
	vulnList, err := h.db.ListVulnerabilities(1000, 0, "", conversationID, "", "")
	if err != nil {
		h.logger.Warn("获取漏洞列表失败", zap.Error(err))
		vulnList = []*database.Vulnerability{}
	}
	vulnerabilities := make([]database.Vulnerability, len(vulnList))
	for i, v := range vulnList {
		vulnerabilities[i] = *v
	}

	// 获取执行结果（从MCP执行记录中获取）
	executionResults := []map[string]interface{}{}
	for _, msg := range messages {
		if len(msg.MCPExecutionIDs) > 0 {
			for _, execID := range msg.MCPExecutionIDs {
				// 尝试从结果存储中获取执行结果
				if h.resultStorage != nil {
					result, err := h.resultStorage.GetResult(execID)
					if err == nil && result != "" {
						// 获取元数据以获取工具名称和创建时间
						metadata, err := h.resultStorage.GetResultMetadata(execID)
						toolName := "unknown"
						createdAt := time.Now()
						if err == nil && metadata != nil {
							toolName = metadata.ToolName
							createdAt = metadata.CreatedAt
						}
						executionResults = append(executionResults, map[string]interface{}{
							"id":        execID,
							"toolName":  toolName,
							"status":    "success",
							"result":    result,
							"createdAt": createdAt.Format(time.RFC3339),
						})
					}
				}
			}
		}
	}

	response := map[string]interface{}{
		"conversationId":   conv.ID,
		"messages":         messages,
		"vulnerabilities":  vulnerabilities,
		"executionResults": executionResults,
	}

	c.JSON(http.StatusOK, response)
}
