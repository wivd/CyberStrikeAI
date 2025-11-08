
// 当前对话ID
let currentConversationId = null;

// 发送消息
async function sendMessage() {
    const input = document.getElementById('chat-input');
    const message = input.value.trim();
    
    if (!message) {
        return;
    }
    
    // 显示用户消息
    addMessage('user', message);
    input.value = '';
    
    // 创建进度消息容器
    const progressId = addMessage('system', '正在处理中...');
    const progressElement = document.getElementById(progressId);
    const progressBubble = progressElement.querySelector('.message-bubble');
    let assistantMessageId = null;
    let mcpExecutionIds = [];
    
    try {
        const response = await fetch('/api/agent-loop/stream', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ 
                message: message,
                conversationId: currentConversationId 
            }),
        });
        
        if (!response.ok) {
            throw new Error('请求失败: ' + response.status);
        }
        
        const reader = response.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        
        while (true) {
            const { done, value } = await reader.read();
            if (done) break;
            
            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split('\n');
            buffer = lines.pop(); // 保留最后一个不完整的行
            
            for (const line of lines) {
                if (line.startsWith('data: ')) {
                    try {
                        const eventData = JSON.parse(line.slice(6));
                        handleStreamEvent(eventData, progressElement, progressBubble, progressId, 
                                         () => assistantMessageId, (id) => { assistantMessageId = id; },
                                         () => mcpExecutionIds, (ids) => { mcpExecutionIds = ids; });
                    } catch (e) {
                        console.error('解析事件数据失败:', e, line);
                    }
                }
            }
        }
        
        // 处理剩余的buffer
        if (buffer.trim()) {
            const lines = buffer.split('\n');
            for (const line of lines) {
                if (line.startsWith('data: ')) {
                    try {
                        const eventData = JSON.parse(line.slice(6));
                        handleStreamEvent(eventData, progressElement, progressBubble, progressId,
                                         () => assistantMessageId, (id) => { assistantMessageId = id; },
                                         () => mcpExecutionIds, (ids) => { mcpExecutionIds = ids; });
                    } catch (e) {
                        console.error('解析事件数据失败:', e, line);
                    }
                }
            }
        }
        
    } catch (error) {
        removeMessage(progressId);
        addMessage('system', '错误: ' + error.message);
    }
}

// 处理流式事件
function handleStreamEvent(event, progressElement, progressBubble, progressId, 
                          getAssistantId, setAssistantId, getMcpIds, setMcpIds) {
    switch (event.type) {
        case 'progress':
            // 更新进度消息
            progressBubble.textContent = event.message;
            break;
            
        case 'tool_call':
            // 显示工具调用信息
            const toolInfo = event.data || {};
            const toolName = toolInfo.toolName || '未知工具';
            const index = toolInfo.index || 0;
            const total = toolInfo.total || 0;
            progressBubble.innerHTML = `🔧 正在调用工具: <strong>${escapeHtml(toolName)}</strong> (${index}/${total})`;
            break;
            
        case 'tool_result':
            // 显示工具执行结果
            const resultInfo = event.data || {};
            const resultToolName = resultInfo.toolName || '未知工具';
            const success = resultInfo.success !== false;
            const statusIcon = success ? '✅' : '❌';
            progressBubble.innerHTML = `${statusIcon} 工具 <strong>${escapeHtml(resultToolName)}</strong> 执行${success ? '完成' : '失败'}`;
            break;
            
        case 'response':
            // 移除进度消息，显示最终回复
            removeMessage(progressId);
            const responseData = event.data || {};
            const mcpIds = responseData.mcpExecutionIds || [];
            setMcpIds(mcpIds);
            
            // 更新对话ID
            if (responseData.conversationId) {
                currentConversationId = responseData.conversationId;
                updateActiveConversation();
            }
            
            // 添加助手回复
            const assistantId = addMessage('assistant', event.message, mcpIds);
            setAssistantId(assistantId);
            
            // 刷新对话列表
            loadConversations();
            break;
            
        case 'error':
            // 显示错误
            removeMessage(progressId);
            addMessage('system', '错误: ' + event.message);
            break;
            
        case 'done':
            // 完成，确保进度消息已移除
            if (progressElement && progressElement.parentNode) {
                removeMessage(progressId);
            }
            // 更新对话ID
            if (event.data && event.data.conversationId) {
                currentConversationId = event.data.conversationId;
                updateActiveConversation();
            }
            break;
    }
}

// 消息计数器，确保ID唯一
let messageCounter = 0;

// 添加消息
function addMessage(role, content, mcpExecutionIds = null) {
    const messagesDiv = document.getElementById('chat-messages');
    const messageDiv = document.createElement('div');
    messageCounter++;
    const id = 'msg-' + Date.now() + '-' + messageCounter + '-' + Math.random().toString(36).substr(2, 9);
    messageDiv.id = id;
    messageDiv.className = 'message ' + role;
    
    // 创建头像
    const avatar = document.createElement('div');
    avatar.className = 'message-avatar';
    if (role === 'user') {
        avatar.textContent = 'U';
    } else if (role === 'assistant') {
        avatar.textContent = 'A';
    } else {
        avatar.textContent = 'S';
    }
    messageDiv.appendChild(avatar);
    
    // 创建消息内容容器
    const contentWrapper = document.createElement('div');
    contentWrapper.className = 'message-content';
    
    // 创建消息气泡
    const bubble = document.createElement('div');
    bubble.className = 'message-bubble';
    
    // 解析 Markdown 格式
    let formattedContent;
    if (typeof marked !== 'undefined') {
        // 使用 marked.js 解析 Markdown
        try {
            // 配置 marked 选项
            marked.setOptions({
                breaks: true,  // 支持换行
                gfm: true,     // 支持 GitHub Flavored Markdown
            });
            formattedContent = marked.parse(content);
        } catch (e) {
            console.error('Markdown 解析失败:', e);
            // 降级处理：转义 HTML 并保留换行
            formattedContent = escapeHtml(content).replace(/\n/g, '<br>');
        }
    } else {
        // 如果没有 marked.js，使用简单处理
        formattedContent = escapeHtml(content).replace(/\n/g, '<br>');
    }
    
    bubble.innerHTML = formattedContent;
    contentWrapper.appendChild(bubble);
    
    // 添加时间戳
    const timeDiv = document.createElement('div');
    timeDiv.className = 'message-time';
    timeDiv.textContent = new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
    contentWrapper.appendChild(timeDiv);
    
    // 如果有MCP执行ID，添加查看详情区域
    if (mcpExecutionIds && Array.isArray(mcpExecutionIds) && mcpExecutionIds.length > 0 && role === 'assistant') {
        const mcpSection = document.createElement('div');
        mcpSection.className = 'mcp-call-section';
        
        const mcpLabel = document.createElement('div');
        mcpLabel.className = 'mcp-call-label';
        mcpLabel.textContent = `工具调用 (${mcpExecutionIds.length})`;
        mcpSection.appendChild(mcpLabel);
        
        const buttonsContainer = document.createElement('div');
        buttonsContainer.className = 'mcp-call-buttons';
        
        mcpExecutionIds.forEach((execId, index) => {
            const detailBtn = document.createElement('button');
            detailBtn.className = 'mcp-detail-btn';
            detailBtn.innerHTML = `<span>调用 #${index + 1}</span>`;
            detailBtn.onclick = () => showMCPDetail(execId);
            buttonsContainer.appendChild(detailBtn);
        });
        
        mcpSection.appendChild(buttonsContainer);
        contentWrapper.appendChild(mcpSection);
    }
    
    messageDiv.appendChild(contentWrapper);
    messagesDiv.appendChild(messageDiv);
    messagesDiv.scrollTop = messagesDiv.scrollHeight;
    return id;
}

// 移除消息
function removeMessage(id) {
    const messageDiv = document.getElementById(id);
    if (messageDiv) {
        messageDiv.remove();
    }
}

// 回车发送消息
document.getElementById('chat-input').addEventListener('keypress', function(e) {
    if (e.key === 'Enter') {
        sendMessage();
    }
});

// 显示MCP调用详情
async function showMCPDetail(executionId) {
    try {
        const response = await fetch(`/api/monitor/execution/${executionId}`);
        const exec = await response.json();
        
        if (response.ok) {
            // 填充模态框内容
            document.getElementById('detail-tool-name').textContent = exec.toolName || 'Unknown';
            document.getElementById('detail-execution-id').textContent = exec.id || 'N/A';
            document.getElementById('detail-status').textContent = getStatusText(exec.status);
            document.getElementById('detail-time').textContent = new Date(exec.startTime).toLocaleString('zh-CN');
            
            // 请求参数
            const requestData = {
                tool: exec.toolName,
                arguments: exec.arguments
            };
            document.getElementById('detail-request').textContent = JSON.stringify(requestData, null, 2);
            
            // 响应结果
            if (exec.result) {
                const responseData = {
                    content: exec.result.content,
                    isError: exec.result.isError
                };
                document.getElementById('detail-response').textContent = JSON.stringify(responseData, null, 2);
                document.getElementById('detail-response').className = exec.result.isError ? 'code-block error' : 'code-block';
            } else {
                document.getElementById('detail-response').textContent = '暂无响应数据';
            }
            
            // 错误信息
            if (exec.error) {
                document.getElementById('detail-error-section').style.display = 'block';
                document.getElementById('detail-error').textContent = exec.error;
            } else {
                document.getElementById('detail-error-section').style.display = 'none';
            }
            
            // 显示模态框
            document.getElementById('mcp-detail-modal').style.display = 'block';
        } else {
            alert('获取详情失败: ' + (exec.error || '未知错误'));
        }
    } catch (error) {
        alert('获取详情失败: ' + error.message);
    }
}

// 关闭MCP详情模态框
function closeMCPDetail() {
    document.getElementById('mcp-detail-modal').style.display = 'none';
}

// 点击模态框外部关闭
window.onclick = function(event) {
    const modal = document.getElementById('mcp-detail-modal');
    if (event.target == modal) {
        closeMCPDetail();
    }
}

// 工具函数
function getStatusText(status) {
    const statusMap = {
        'pending': '等待中',
        'running': '执行中',
        'completed': '已完成',
        'failed': '失败'
    };
    return statusMap[status] || status;
}

function formatDuration(ms) {
    const seconds = Math.floor(ms / 1000);
    const minutes = Math.floor(seconds / 60);
    const hours = Math.floor(minutes / 60);
    
    if (hours > 0) {
        return `${hours}小时${minutes % 60}分钟`;
    } else if (minutes > 0) {
        return `${minutes}分钟${seconds % 60}秒`;
    } else {
        return `${seconds}秒`;
    }
}

function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// 开始新对话
function startNewConversation() {
    currentConversationId = null;
    document.getElementById('chat-messages').innerHTML = '';
    addMessage('assistant', '系统已就绪。请输入您的测试需求，系统将自动执行相应的安全测试。');
    updateActiveConversation();
}

// 加载对话列表
async function loadConversations() {
    try {
        const response = await fetch('/api/conversations?limit=50');
        const conversations = await response.json();
        
        const listContainer = document.getElementById('conversations-list');
        listContainer.innerHTML = '';
        
        if (conversations.length === 0) {
            listContainer.innerHTML = '<div style="padding: 20px; text-align: center; color: var(--text-muted); font-size: 0.875rem;">暂无历史对话</div>';
            return;
        }
        
        conversations.forEach(conv => {
            const item = document.createElement('div');
            item.className = 'conversation-item';
            item.dataset.conversationId = conv.id;
            if (conv.id === currentConversationId) {
                item.classList.add('active');
            }
            
            const title = document.createElement('div');
            title.className = 'conversation-title';
            title.textContent = conv.title || '未命名对话';
            item.appendChild(title);
            
            const time = document.createElement('div');
            time.className = 'conversation-time';
            // 解析时间，支持多种格式
            let dateObj;
            if (conv.updatedAt) {
                dateObj = new Date(conv.updatedAt);
                // 检查日期是否有效
                if (isNaN(dateObj.getTime())) {
                    // 如果解析失败，尝试其他格式
                    console.warn('时间解析失败:', conv.updatedAt);
                    dateObj = new Date();
                }
            } else {
                dateObj = new Date();
            }
            
            // 格式化时间显示
            const now = new Date();
            const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
            const yesterday = new Date(today);
            yesterday.setDate(yesterday.getDate() - 1);
            const messageDate = new Date(dateObj.getFullYear(), dateObj.getMonth(), dateObj.getDate());
            
            let timeText;
            if (messageDate.getTime() === today.getTime()) {
                // 今天：只显示时间
                timeText = dateObj.toLocaleTimeString('zh-CN', {
                    hour: '2-digit',
                    minute: '2-digit'
                });
            } else if (messageDate.getTime() === yesterday.getTime()) {
                // 昨天
                timeText = '昨天 ' + dateObj.toLocaleTimeString('zh-CN', {
                    hour: '2-digit',
                    minute: '2-digit'
                });
            } else if (now.getFullYear() === dateObj.getFullYear()) {
                // 今年：显示月日和时间
                timeText = dateObj.toLocaleString('zh-CN', {
                    month: 'short',
                    day: 'numeric',
                    hour: '2-digit',
                    minute: '2-digit'
                });
            } else {
                // 去年或更早：显示完整日期和时间
                timeText = dateObj.toLocaleString('zh-CN', {
                    year: 'numeric',
                    month: 'short',
                    day: 'numeric',
                    hour: '2-digit',
                    minute: '2-digit'
                });
            }
            
            time.textContent = timeText;
            item.appendChild(time);
            
            item.onclick = () => loadConversation(conv.id);
            listContainer.appendChild(item);
        });
    } catch (error) {
        console.error('加载对话列表失败:', error);
    }
}

// 加载对话
async function loadConversation(conversationId) {
    try {
        const response = await fetch(`/api/conversations/${conversationId}`);
        const conversation = await response.json();
        
        if (!response.ok) {
            alert('加载对话失败: ' + (conversation.error || '未知错误'));
            return;
        }
        
        // 更新当前对话ID
        currentConversationId = conversationId;
        updateActiveConversation();
        
        // 清空消息区域
        const messagesDiv = document.getElementById('chat-messages');
        messagesDiv.innerHTML = '';
        
        // 加载消息
        if (conversation.messages && conversation.messages.length > 0) {
            conversation.messages.forEach(msg => {
                addMessage(msg.role, msg.content, msg.mcpExecutionIds || []);
            });
        } else {
            addMessage('assistant', '系统已就绪。请输入您的测试需求，系统将自动执行相应的安全测试。');
        }
        
        // 滚动到底部
        messagesDiv.scrollTop = messagesDiv.scrollHeight;
        
        // 刷新对话列表
        loadConversations();
    } catch (error) {
        console.error('加载对话失败:', error);
        alert('加载对话失败: ' + error.message);
    }
}

// 更新活动对话样式
function updateActiveConversation() {
    document.querySelectorAll('.conversation-item').forEach(item => {
        item.classList.remove('active');
        if (currentConversationId && item.dataset.conversationId === currentConversationId) {
            item.classList.add('active');
        }
    });
}

// 页面加载时初始化
document.addEventListener('DOMContentLoaded', function() {
    // 加载对话列表
    loadConversations();
    
    // 添加欢迎消息
    addMessage('assistant', '系统已就绪。请输入您的测试需求，系统将自动执行相应的安全测试。');
});

