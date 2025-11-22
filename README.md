# llm-mcp-rag

基于 Go 的 LLM + MCP + RAG 演示项目，展示如何利用火山方舟 Doubao 模型协调多种 MCP 工具，完成“抓取公开网页内容 → 总结 → 写文件”的自动化流程。

## 项目结构
- `main.go`：示例入口，注册 fetch 与 filesystem 两个 MCP 客户端，构造 `Agent` 并触发一次任务。自动检测并启用 RAG 功能。
- `agent.go`：封装代理循环，处理 LLM 返回的工具调用并调用对应 MCP 工具。包含 RAG 检索集成。
- `chat_open_ai.go`：对接 Ark Doubao ChatCompletion API，维护消息历史、系统提示、RAG 上下文及工具声明。
- `mcp_client.go`：MCP 客户端封装，负责与 MCP 服务器建立连接、初始化、工具发现和调用。
- `rag.go`：RAG 检索模块，实现从本地知识库检索相关文档的功能。
- `consts.go`：集中定义 Ark 相关环境变量名。
- `knowledge_base/`：知识库目录，存放用于 RAG 检索的文档文件（.txt、.md 格式）。
- `new.md`：运行 demo 后生成/覆盖的总结文件。

## 知识点框架

### 核心技术栈

#### 1. Go 语言生态
- **版本要求**：Go 1.21+
- **并发模型**：利用 Go 的 goroutine 和 channel 处理并发任务
- **标准库**：使用 `context` 进行上下文管理和取消传播，`encoding/json` 处理 JSON 序列化/反序列化
- **错误处理**：采用 Go 惯用的显式错误返回模式

#### 2. MCP (Model Context Protocol)
- **协议定位**：由 Anthropic 提出的标准化协议，用于 LLM 与外部工具的标准通信
- **设计目标**：解耦 LLM 应用与工具实现，提供统一的工具接入接口
- **协议版本**：使用 `LATEST_PROTOCOL_VERSION` 确保兼容性
- **传输层**：基于 stdio 的 JSON-RPC 2.0 通信，无需网络配置
- **核心能力**：
  - 工具发现：动态获取可用工具列表
  - 工具调用：标准化的工具执行接口
  - 资源管理：支持资源列表和访问（本项目未使用）

#### 3. 火山方舟 Doubao 模型
- **API 兼容性**：完全兼容 OpenAI ChatCompletion API
- **模型标识**：使用模型名称（如 `doubao-seed-1-6-251015`）指定具体模型版本
- **功能支持**：
  - 函数调用（Function Calling）：支持工具调用格式
  - 流式响应：支持流式输出（本项目使用非流式）
  - 上下文管理：支持长上下文对话

#### 4. MCP Go SDK (`github.com/mark3labs/mcp-go`)
- **核心组件**：
  - `client.Client`：MCP 客户端实现，管理连接生命周期
  - `transport.Stdio`：stdio 传输层实现，处理进程间通信
  - `mcp` 包：定义 MCP 协议的数据结构和请求/响应格式
- **关键方法**：
  - `Initialize`：初始化 MCP 连接，交换客户端/服务器信息
  - `ListTools`：获取服务器提供的工具列表
  - `CallTool`：执行工具调用并返回结果

#### 5. OpenAI Go SDK (`github.com/sashabaranov/go-openai`)
- **作用**：作为通用 SDK，兼容所有 OpenAI API 格式的服务
- **核心功能**：
  - `Client.CreateChatCompletion`：发送聊天完成请求
  - `ChatCompletionRequest`：封装请求参数（模型、消息、工具等）
  - `ChatCompletionMessage`：消息结构，支持 System/User/Assistant/Tool 角色

### 架构设计模式

#### 1. Agent 模式（协调者模式）
- **设计理念**：Agent 作为智能协调中心，统一管理 LLM 和多个工具的生命周期
- **核心组件**：
  ```go
  type Agent struct {
      MCPClient    []*MCPClient  // MCP 客户端列表
      LLM          *ChatOpenAI   // LLM 交互封装
      Model        string         // 模型标识
      SystemPrompt string         // 系统提示词
      Ctx          context.Context // 上下文
      RAGCtx       string         // RAG 上下文
  }
  ```
- **职责分离**：
  - **Agent**：
    - 初始化阶段：启动所有 MCP 客户端，收集工具列表，初始化 LLM
    - 执行阶段：处理工具调用循环，匹配工具并执行
    - 清理阶段：统一关闭所有资源
  - **ChatOpenAI**：
    - 消息历史管理：维护完整的对话上下文链
    - 工具声明转换：将 MCP 工具格式转换为 OpenAI 格式
    - API 调用封装：处理与 LLM 服务的交互细节
  - **MCPClient**：
    - 进程管理：启动和管理 MCP 服务器子进程
    - 协议通信：处理 JSON-RPC 请求/响应
    - 工具封装：提供工具发现和调用的统一接口

#### 2. 工具调用循环 (Tool Calling Loop)
- **循环机制**：实现多轮工具调用的自动迭代
- **详细流程**：
  ```
  1. 用户输入 Prompt
     ↓
  2. LLM 分析并生成响应
     ├─ 如果包含工具调用 → 进入步骤 3
     └─ 如果无工具调用 → 返回最终结果，结束
     ↓
  3. Agent 解析工具调用请求
     ├─ 遍历所有 MCP 客户端
     ├─ 匹配工具名称
     └─ 执行对应的工具
     ↓
  4. 工具执行结果追加到消息历史
     ├─ 角色：Tool
     ├─ 关联：ToolCallID
     └─ 内容：工具返回的文本
     ↓
  5. 再次调用 LLM（空 prompt）
     └─ 回到步骤 2，继续循环
  ```
- **关键特性**：
  - **上下文累积**：每次工具调用结果都会追加到消息历史，形成完整的执行链
  - **自动迭代**：无需手动判断，LLM 自动决定是否需要继续调用工具
  - **错误容错**：单个工具调用失败不影响其他工具的执行

#### 3. 适配器模式 (Adapter Pattern)
- **转换场景**：MCP 工具定义 → OpenAI 函数定义
- **转换函数**：`MCPTool2ArkTool`
- **转换细节**：
  ```go
  MCP Tool 结构：
  {
      Name: "fetch_url",
      Description: "获取网页内容",
      InputSchema: {
          Type: "object",
          Properties: {...},
          Required: [...]
      }
  }
  
  转换为 OpenAI Tool 结构：
  {
      Type: "function",
      Function: {
          Name: "fetch_url",
          Description: "获取网页内容",
          Parameters: {
              type: "object",
              properties: {...},
              required: [...]
          }
      }
  }
  ```
- **类型处理**：
  - 自动处理空类型，默认设置为 "object"
  - 保持 JSON Schema 的完整结构
  - 确保参数验证规则的一致性

#### 4. 选项模式 (Options Pattern)
- **实现方式**：使用函数式选项模式配置 `ChatOpenAI`
- **选项函数**：
  - `WithSystemPrompt`：设置系统提示词
  - `WithRagContext`：注入 RAG 上下文
  - `WithLLMTools`：注册工具列表
  - `WithMessage`：预设消息历史
- **优势**：
  - 灵活的配置方式，支持可选参数
  - 易于扩展新的配置选项
  - 代码可读性强

### 关键概念详解

#### 1. MCP (Model Context Protocol) 深度解析

##### 协议架构
- **分层设计**：
  - **传输层**：stdio（标准输入输出），也可扩展为 HTTP/WebSocket
  - **协议层**：JSON-RPC 2.0，定义请求/响应格式
  - **应用层**：工具定义、资源管理、提示词模板等

##### 初始化流程
```go
1. 创建 stdio 传输：transport.NewStdio(cmd, env, args...)
   └─ 启动子进程（如：uvx mcp-server-fetch）
   
2. 创建客户端：client.NewClient(stdioTransport)
   └─ 建立 JSON-RPC 通信通道
   
3. 启动连接：client.Start(ctx)
   └─ 启动传输层的读写 goroutine
   
4. 初始化协议：client.Initialize(ctx, request)
   └─ 发送初始化请求：
      {
          "jsonrpc": "2.0",
          "method": "initialize",
          "params": {
              "protocolVersion": "2024-11-05",
              "clientInfo": {
                  "name": "example-client",
                  "version": "0.0.1"
              }
          }
      }
   └─ 接收服务器能力信息
```

##### 工具发现机制
- **请求格式**：
  ```json
  {
      "jsonrpc": "2.0",
      "method": "tools/list",
      "id": 1
  }
  ```
- **响应结构**：
  ```json
  {
      "jsonrpc": "2.0",
      "result": {
          "tools": [
              {
                  "name": "fetch_url",
                  "description": "获取指定 URL 的内容",
                  "inputSchema": {
                      "type": "object",
                      "properties": {
                          "url": {
                              "type": "string",
                              "description": "要获取的 URL"
                          }
                      },
                      "required": ["url"]
                  }
              }
          ]
      }
  }
  ```

##### 工具调用机制
- **请求格式**：
  ```json
  {
      "jsonrpc": "2.0",
      "method": "tools/call",
      "params": {
          "name": "fetch_url",
          "arguments": {
              "url": "https://example.com"
          }
      }
  }
  ```
- **响应结构**：
  ```json
  {
      "jsonrpc": "2.0",
      "result": {
          "content": [
              {
                  "type": "text",
                  "text": "网页内容..."
              }
          ]
      }
  }
  ```

#### 2. RAG (Retrieval-Augmented Generation) 实现

##### 实现原理
本项目实现了基于关键词匹配的 RAG 检索系统，能够从本地知识库中检索相关文档并注入到 LLM 上下文中。

**核心流程**：
1. **文档加载**：从 `knowledge_base` 目录加载所有 `.txt`、`.md`、`.go` 文件
2. **查询分析**：对用户查询进行分词处理
3. **相关性计算**：计算每个文档与查询的相关性分数
4. **文档选择**：选择分数最高的前 N 个文档（默认 3 个）
5. **上下文构建**：将选中的文档内容格式化后注入到 LLM 消息历史

##### 代码实现

**RAG 检索器** (`rag.go`)：
```go
type RAGRetriever struct {
    KnowledgeBaseDir string  // 知识库目录路径
    MaxResults       int     // 最大返回文档数（默认3）
    MinScore         float64 // 最低相关性分数（默认0.1）
}

// 检索相关文档
func (r *RAGRetriever) Retrieve(ctx context.Context, query string) (string, error)
```

**Agent 集成**：
```go
// 创建带 RAG 功能的 Agent
agent := NewAgentWithRAG(ctx, model, mcpClients, systemPrompt, knowledgeBaseDir)

// 使用 RAG 检索执行任务
result := agent.InvokeWithRAG(userPrompt)
```

**自动检索流程**：
- `InvokeWithRAG` 方法会在执行任务前自动进行 RAG 检索
- 检索到的上下文会作为 `User` 角色的消息注入到 LLM
- LLM 可以基于这些上下文信息更好地理解任务需求

##### 相关性评分算法

1. **基础分数**：匹配的查询词数量 / 总查询词数量（权重 70%）
2. **词频分数**：匹配词在文档中的出现频率（权重 30%）
3. **综合分数**：`finalScore = baseScore * 0.7 + freqScore * 0.3`

##### 使用方式

1. **创建知识库目录**：
   ```bash
   mkdir -p knowledge_base
   ```

2. **添加知识文档**：
   在 `knowledge_base` 目录下放置 `.txt` 或 `.md` 文件，例如：
   - `web_scraping_guide.txt` - 网页抓取指南
   - `content_summarization.txt` - 内容摘要指南
   - `mcp_tools.txt` - MCP 工具使用说明

3. **自动启用**：
   如果 `knowledge_base` 目录存在，程序会自动启用 RAG 功能

4. **示例输出**：
   ```
   Knowledge base found, RAG enabled: /path/to/knowledge_base
   Retrieving relevant context from knowledge base...
   RAG context retrieved, injecting into LLM...
   ```

##### 与传统 RAG 的区别

- **传统 RAG**：向量化 → 向量数据库 → 相似度搜索 → 注入上下文
- **本项目实现**：关键词匹配 → 相关性评分 → 文档选择 → 注入上下文
- **优势**：
  - 无需向量数据库，实现简单
  - 适合小到中等规模的知识库
  - 易于理解和调试
- **扩展方向**：
  - 可集成向量数据库（如 Milvus、Pinecone）实现语义搜索
  - 可添加更复杂的 NLP 处理（如 TF-IDF、BM25）
  - 可支持多种文档格式（PDF、Word 等）

#### 3. 工具声明与调用流程

##### 工具发现阶段
1. **启动 MCP 服务器**：
   - `mcp-server-fetch`：通过 `uvx` 启动 Python 实现的 fetch 服务器
   - `@modelcontextprotocol/server-filesystem`：通过 `npx` 启动 Node.js 实现的文件系统服务器
2. **初始化连接**：建立 stdio 通信通道
3. **获取工具列表**：调用 `ListTools` 获取服务器提供的所有工具
4. **工具注册**：将工具列表转换为 LLM 可理解的格式

##### 工具注册阶段
- **转换过程**：
  ```
  MCP Tool (服务器定义)
    ↓ MCPTool2ArkTool()
  OpenAI Tool (LLM 可理解)
    ↓ 添加到 ChatCompletionRequest.Tools
  LLM 函数定义（模型内部）
  ```
- **注册时机**：在 `NewChatOpenAI` 初始化时完成

##### 工具执行阶段
1. **LLM 决策**：根据用户需求和工具能力，决定调用哪些工具
2. **工具调用请求**：LLM 返回 `ToolCalls` 数组
3. **工具匹配**：
   ```go
   for _, toolCall := range toolCalls {
       for _, mcpClient := range mcpClients {
           for _, mcpTool := range mcpClient.GetTool() {
               if mcpTool.Name == toolCall.Function.Name {
                   // 找到匹配的工具，执行调用
               }
           }
       }
   }
   ```
4. **参数处理**：
   - LLM 返回的参数是 JSON 字符串
   - 需要解析为 `map[string]any` 格式
   - 支持字符串和 map 两种输入格式
5. **结果提取**：从 MCP 返回的 `Content` 数组中提取文本内容

#### 4. 消息历史管理

##### 消息类型详解
- **System Message**：
  - **作用**：定义 Agent 的行为规则和约束
  - **位置**：消息数组的第一条
  - **内容**：系统提示词，指导 LLM 如何使用工具
- **User Message**：
  - **类型1**：用户输入的 Prompt
  - **类型2**：RAG 上下文（可选）
  - **追加时机**：每次用户输入和 RAG 注入时
- **Assistant Message**：
  - **内容**：LLM 的回复文本（如果有）
  - **工具调用**：包含 `ToolCalls` 数组（如果需要调用工具）
  - **追加时机**：每次 LLM 响应后
- **Tool Message**：
  - **角色**：`ChatMessageRoleTool`
  - **关联**：通过 `ToolCallID` 关联到对应的工具调用
  - **内容**：工具执行的返回结果
  - **追加时机**：每次工具调用完成后

##### 上下文维护机制
- **顺序保证**：消息按时间顺序追加，形成完整的对话链
- **上下文窗口**：LLM 可以看到完整的消息历史
- **工具调用链**：工具调用和结果成对出现，保持上下文连贯性

##### 消息流转示例
```
[System] "你是一个内容获取与文件写入助手..."
[User] "访问 https://example.com 并总结"
[Assistant] (工具调用: fetch_url)
[Tool] "网页内容：..."
[Assistant] "根据获取的内容，总结如下：..."
[Tool] (工具调用: write_to_file)
[Tool] "文件写入成功"
[Assistant] "任务完成，已写入文件"
```

### 技术实现细节

#### 1. MCP 客户端管理

##### 多客户端架构
- **设计模式**：每个 MCP 服务器对应一个 `MCPClient` 实例
- **客户端列表**：`Agent.MCPClient []*MCPClient`
- **初始化流程**：
  ```go
  for _, mcpClient := range mcpClients {
      // 1. 启动 stdio 传输
      err := mcpClient.Start()
      
      // 2. 获取工具列表
      err = mcpClient.SetTools()
      
      // 3. 收集工具到统一列表
      tools = append(tools, mcpClient.GetTool()...)
  }
  ```

##### 生命周期管理
- **启动阶段**：
  - 创建 stdio 传输，启动子进程
  - 建立 JSON-RPC 连接
  - 发送初始化请求
  - 获取工具列表
- **运行阶段**：
  - 保持连接活跃
  - 处理工具调用请求
  - 管理消息队列
- **关闭阶段**：
  - 调用 `Client.Close()` 关闭连接
  - 终止子进程
  - 清理资源

##### 错误处理策略
- **启动失败**：单个客户端失败不影响其他客户端，继续初始化
- **工具获取失败**：跳过该客户端的工具，不影响整体功能
- **工具调用失败**：记录错误，继续处理其他工具调用

#### 2. stdio 传输协议

##### 进程通信机制
- **启动方式**：
  ```go
  transport.NewStdio("uvx", nil, []string{"mcp-server-fetch"})
  // 等价于命令行：uvx mcp-server-fetch
  ```
- **通信通道**：
  - **stdin**：客户端 → 服务器（发送 JSON-RPC 请求）
  - **stdout**：服务器 → 客户端（接收 JSON-RPC 响应）
  - **stderr**：服务器日志输出（不影响通信）

##### JSON-RPC 2.0 协议
- **请求格式**：
  ```json
  {
      "jsonrpc": "2.0",
      "method": "tools/call",
      "params": {...},
      "id": 1
  }
  ```
- **响应格式**：
  ```json
  {
      "jsonrpc": "2.0",
      "result": {...},
      "id": 1
  }
  ```
- **错误格式**：
  ```json
  {
      "jsonrpc": "2.0",
      "error": {
          "code": -32600,
          "message": "Invalid Request"
      },
      "id": 1
  }
  ```

##### 并发处理
- **读写分离**：使用独立的 goroutine 处理 stdin 写入和 stdout 读取
- **请求队列**：维护请求 ID 映射，确保响应正确匹配
- **上下文传播**：通过 `context.Context` 实现请求取消和超时控制

#### 3. 工具参数处理

##### 参数格式转换
- **LLM 返回格式**：JSON 字符串
  ```json
  "{\"url\": \"https://example.com\"}"
  ```
- **MCP 所需格式**：`map[string]any`
  ```go
  map[string]any{
      "url": "https://example.com"
  }
  ```
- **转换逻辑**：
  ```go
  switch v := args.(type) {
  case string:
      json.Unmarshal([]byte(v), &arguments)
  case map[string]any:
      arguments = v
  }
  ```

##### 类型验证
- **JSON Schema 验证**：MCP 服务器根据工具定义的 `InputSchema` 验证参数
- **必需参数检查**：确保 `required` 字段列表中的所有参数都已提供
- **类型匹配**：确保参数类型与 schema 定义一致

##### 结果提取
- **Content 结构**：
  ```go
  type Content struct {
      Type string `json:"type"` // "text" | "image" | ...
      Text string `json:"text,omitempty"`
      // ...
  }
  ```
- **文本提取**：使用 `mcp.GetTextFromContent()` 从 Content 数组中提取所有文本内容

#### 4. 可扩展性设计

##### 工具扩展机制
- **新增工具步骤**：
  1. 安装或开发新的 MCP 服务器
  2. 在 `main.go` 中创建新的 `MCPClient`
  3. 添加到 `Agent` 的客户端列表
  4. 无需修改核心代码，工具自动被发现和注册

##### 模型切换
- **配置方式**：修改 `main.go` 中的 `doubaoModel` 常量
- **兼容性**：只要模型支持 OpenAI 兼容的 Function Calling，即可无缝切换
- **示例**：
  ```go
  const doubaoModel = "doubao-seed-1-6-251015"  // 当前模型
  // 可切换为其他模型，如 "gpt-4", "claude-3", 等
  ```

##### 配置灵活性
- **环境变量**：
  - `ARK_API_KEY`：必需，API 密钥
  - `ARK_BASE_URL`：可选，自定义 API 网关地址
- **系统提示**：可在 `main.go` 中自定义系统提示词，控制 Agent 行为
- **RAG 上下文**：支持动态注入额外的上下文信息

## 工作流程
1. 启动全部 MCP 客户端（如 `mcp-server-fetch`、`@modelcontextprotocol/server-filesystem`），收集它们暴露的工具列表。
2. 将工具列表与系统提示、RAG 上下文一起传给 Doubao 模型。
3. LLM 按需发起工具调用，`Agent` 捕获调用并实际请求 MCP 工具。
4. 工具返回内容被追加到对话历史中，直至 LLM 产生最终回答。

## 启动过程详解

### 完整启动流程

#### 阶段一：程序初始化
1. **环境检查**
   - 检查 `ARK_API_KEY` 环境变量是否存在
   - 如果不存在，程序 panic 并提示错误
   - 可选检查 `ARK_BASE_URL`，如果未设置则使用默认值

2. **上下文创建**
   ```go
   ctx := context.Background()
   ```
   - 创建根上下文，用于管理整个程序的生命周期
   - 支持后续的取消和超时控制

3. **系统提示词定义**
   - 定义 Agent 的行为规则和约束
   - 明确指定可用的工具和任务要求
   - 示例：限制只能使用 MCP 工具，不能自行访问网络

#### 阶段二：MCP 客户端创建与启动

##### 步骤 1：创建 Fetch MCP 客户端
```go
fetchMcpCli := NewMCPClient(ctx, "uvx", nil, []string{"mcp-server-fetch"})
```
- **命令**：`uvx`（Python 包执行器）
- **参数**：`mcp-server-fetch`（MCP 服务器包名）
- **作用**：创建用于网页抓取的 MCP 客户端
- **内部过程**：
  1. 创建 `transport.Stdio` 传输层
  2. 创建 `client.Client` 实例
  3. 返回 `MCPClient` 结构体（此时尚未启动）

##### 步骤 2：创建 Filesystem MCP 客户端
```go
fileMcpCli := NewMCPClient(ctx, "npx", nil, []string{
    "-y", 
    "@modelcontextprotocol/server-filesystem", 
    allowDir
})
```
- **命令**：`npx`（Node.js 包执行器）
- **参数**：
  - `-y`：自动确认安装
  - `@modelcontextprotocol/server-filesystem`：MCP 服务器包名
  - `allowDir`：允许访问的目录路径（当前工作目录）
- **作用**：创建用于文件系统操作的 MCP 客户端

##### 步骤 3：创建 Agent 并启动所有客户端
```go
agent := NewAgent(ctx, doubaoModel, []*MCPClient{
    fetchMcpCli, 
    fileMcpCli
}, systemPrompt, "")
```

**Agent 初始化详细流程**：

1. **启动所有 MCP 客户端**
   ```go
   for _, item := range mcpCli {
       // 启动 stdio 传输
       err := item.Start()
   ```
   - 对每个客户端调用 `Start()` 方法
   - **内部执行**：
     - 调用 `client.Start(ctx)` 启动传输层
     - 创建子进程（如：`uvx mcp-server-fetch`）
     - 启动 goroutine 处理 stdin/stdout 通信
     - 建立 JSON-RPC 通信通道

2. **初始化 MCP 协议**
   ```go
   mcpInitReq := mcp.InitializeRequest{}
   mcpInitReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
   mcpInitReq.Params.ClientInfo = mcp.Implementation{
       Name:    "example-client",
       Version: "0.0.1",
   }
   client.Initialize(ctx, mcpInitReq)
   ```
   - 发送初始化请求到 MCP 服务器
   - 交换协议版本和客户端信息
   - 接收服务器能力信息

3. **获取工具列表**
   ```go
   err = item.SetTools()
   ```
   - 调用 `ListTools` 方法获取服务器提供的工具
   - **请求**：
     ```json
     {
         "jsonrpc": "2.0",
         "method": "tools/list",
         "id": 1
     }
     ```
   - **响应处理**：
     - 解析工具列表
     - 提取工具名称、描述、输入模式
     - 存储到 `MCPClient.Tools` 数组

4. **工具注册日志**
   ```go
   for _, t := range item.GetTool() {
       fmt.Println("tool ready:", t.Name)
   }
   ```
   - 打印每个已注册的工具名称
   - 便于调试和确认工具是否正确加载

5. **收集所有工具**
   ```go
   tools = append(tools, item.GetTool()...)
   ```
   - 将所有客户端的工具合并到统一列表
   - 准备传递给 LLM

6. **初始化 LLM 客户端**
   ```go
   llm := NewChatOpenAI(ctx, model, 
       WithSystemPrompt(systemPrompt), 
       WithLLMTools(tools), 
       WithRagContext(ragCtx))
   ```
   - **内部过程**：
     1. 读取环境变量 `ARK_API_KEY` 和 `ARK_BASE_URL`
     2. 创建 OpenAI 兼容的客户端配置
     3. 设置 API 基础 URL（默认或自定义）
     4. 创建 `ChatOpenAI` 实例
     5. 添加系统提示词到消息历史
     6. 添加 RAG 上下文（如果提供）
     7. 转换工具列表格式（MCP → OpenAI）

7. **返回 Agent 实例**
   ```go
   return &Agent{
       MCPClient:    mcpCli,
       LLM:          llm,
       Model:        model,
       SystemPrompt: systemPrompt,
       RAGCtx:       ragCtx,
   }
   ```

#### 阶段三：任务执行

##### 步骤 1：调用 Agent.Invoke()
```go
result := agent.Invoke("访问 https://www.sina.com.cn/ 首页公开内容...")
```

##### 步骤 2：LLM 首次调用
```go
response, toolCalls := a.LLM.Chat(prompt)
```
- **内部流程**：
  1. 将用户 prompt 追加到消息历史
  2. 构建 `ChatCompletionRequest`：
     - Model: 模型名称
     - Messages: 完整的消息历史
     - Tools: 转换后的工具列表
  3. 调用 `client.CreateChatCompletion()`
  4. 发送 HTTP 请求到火山方舟 API
  5. 解析响应，提取 Assistant 消息
  6. 追加 Assistant 消息到历史
  7. 返回响应文本和工具调用列表

##### 步骤 3：工具调用循环
```go
for len(toolCalls) > 0 {
    // 执行工具调用
    for _, toolCall := range toolCalls {
        // 匹配工具
        for _, mcpClient := range a.MCPClient {
            for _, mcpTool := range mcpClient.GetTool() {
                if mcpTool.Name == toolCall.Function.Name {
                    // 执行工具
                    toolText, err := mcpClient.CallTool(
                        toolCall.Function.Name, 
                        toolCall.Function.Arguments
                    )
                    // 追加结果到消息历史
                    a.LLM.Message = append(a.LLM.Message, ...)
                }
            }
        }
    }
    // 继续对话
    response, toolCalls = a.LLM.Chat("")
}
```

**工具调用详细流程**：
1. **工具匹配**：
   - 遍历所有 MCP 客户端
   - 在每个客户端中查找匹配的工具名称
   - 找到后执行调用

2. **参数处理**：
   - LLM 返回的参数是 JSON 字符串
   - 解析为 `map[string]any` 格式
   - 传递给 MCP 客户端

3. **工具执行**：
   ```go
   res, err := m.Client.CallTool(ctx, mcp.CallToolRequest{
       Params: mcp.CallToolParams{
           Name:      name,
           Arguments: arguments,
       },
   })
   ```
   - 发送 JSON-RPC 请求到 MCP 服务器
   - 等待服务器执行工具
   - 接收执行结果

4. **结果提取**：
   ```go
   return mcp.GetTextFromContent(res.Content), nil
   ```
   - 从 Content 数组中提取文本内容
   - 返回给 Agent

5. **结果追加**：
   ```go
   a.LLM.Message = append(a.LLM.Message, ark.ChatCompletionMessage{
       Role:       ark.ChatMessageRoleTool,
       Content:    toolText,
       ToolCallID: toolCall.ID,
   })
   ```
   - 将工具结果作为 Tool 消息追加
   - 通过 `ToolCallID` 关联到原始调用

6. **继续对话**：
   - 使用空 prompt 再次调用 LLM
   - LLM 基于工具结果生成下一步响应
   - 如果还有工具调用，继续循环
   - 如果没有工具调用，返回最终结果

#### 阶段四：资源清理

##### 关闭所有 MCP 客户端
```go
a.Close()
```
- **内部执行**：
  ```go
  for _, mcpClient := range a.MCPClient {
      mcpClient.Close()
  }
  ```
  - 调用每个客户端的 `Close()` 方法
  - 关闭 JSON-RPC 连接
  - 终止子进程
  - 清理资源

##### 返回最终结果
```go
return response
```
- 返回 LLM 的最终回复
- 打印到控制台

### 启动时序图

```
main()
  │
  ├─> 创建 Context
  ├─> 定义 SystemPrompt
  ├─> 创建 fetchMcpCli (NewMCPClient)
  ├─> 创建 fileMcpCli (NewMCPClient)
  │
  └─> NewAgent()
      │
      ├─> fetchMcpCli.Start()
      │   ├─> 启动子进程 (uvx mcp-server-fetch)
      │   ├─> 建立 stdio 连接
      │   └─> Initialize() 协议初始化
      │
      ├─> fetchMcpCli.SetTools()
      │   └─> ListTools() 获取工具列表
      │
      ├─> fileMcpCli.Start()
      │   ├─> 启动子进程 (npx @modelcontextprotocol/server-filesystem)
      │   ├─> 建立 stdio 连接
      │   └─> Initialize() 协议初始化
      │
      ├─> fileMcpCli.SetTools()
      │   └─> ListTools() 获取工具列表
      │
      └─> NewChatOpenAI()
          ├─> 读取环境变量
          ├─> 创建 OpenAI 客户端
          ├─> 添加系统提示词
          ├─> 转换工具列表格式
          └─> 返回 LLM 实例
      │
      └─> 返回 Agent 实例
  │
  └─> agent.Invoke(prompt)
      │
      ├─> LLM.Chat(prompt)
      │   ├─> 发送请求到火山方舟 API
      │   └─> 返回响应和工具调用
      │
      ├─> [循环] 执行工具调用
      │   ├─> 匹配工具
      │   ├─> CallTool() 执行工具
      │   ├─> 追加结果到消息历史
      │   └─> LLM.Chat("") 继续对话
      │
      └─> agent.Close()
          └─> 关闭所有 MCP 客户端
```

### 启动日志示例

```
allowDir: /home/user/goproject/llm-mcp-rag
tool ready: fetch_url
tool ready: write_to_file
init LLM & Tools
init LLM successfully
init chat...
toolCalls [{ID: call_xxx Function: {Name: fetch_url Arguments: {"url":"https://www.sina.com.cn/"}}}]
tool use call_xxx fetch_url {"url":"https://www.sina.com.cn/"}
response 
init chat...
toolCalls [{ID: call_yyy Function: {Name: write_to_file Arguments: {"path":"new.md","content":"..."}}}]
tool use call_yyy write_to_file {"path":"new.md","content":"..."}
response 任务完成，已成功将摘要写入 new.md 文件
all close
result: 任务完成，已成功将摘要写入 new.md 文件
```

## 环境准备
1. Go 1.21+。
2. 火山方舟凭证：设置 `ARK_API_KEY`（必需），如需自定义网关可额外设置 `ARK_BASE_URL`。
3. 外部依赖：需已安装 `uv`（运行 `mcp-server-fetch`）和 `node`/`npx`（运行 `@modelcontextprotocol/server-filesystem`）。

### Ubuntu 22.04 快速安装


**方式二：手动安装**
详细步骤请参考 [UBUNTU_SETUP.md](./UBUNTU_SETUP.md)

## RAG 功能使用指南

### 快速开始

1. **创建知识库目录**（如果不存在）：
   ```bash
   mkdir -p knowledge_base
   ```

2. **添加知识文档**：
   在 `knowledge_base` 目录下放置文本文件（支持 `.txt`、`.md`、`.go` 格式）：
   ```bash
   # 示例：添加网页抓取指南
   echo "网页抓取最佳实践：遵守 robots.txt，设置合理请求间隔..." > knowledge_base/web_scraping_guide.txt
   ```

3. **运行程序**：
   程序会自动检测 `knowledge_base` 目录，如果存在则启用 RAG 功能：
   ```bash
   go run main.go
   ```

### RAG 工作流程

```
用户查询
    ↓
RAG 检索器分析查询
    ↓
从 knowledge_base 加载所有文档
    ↓
计算文档与查询的相关性分数
    ↓
选择分数最高的前 3 个文档
    ↓
构建 RAG 上下文（包含文档内容和来源）
    ↓
注入到 LLM 消息历史
    ↓
LLM 基于上下文执行任务
```

### 自定义配置

可以通过修改 `rag.go` 中的 `RAGRetriever` 结构体来调整检索行为：

```go
retriever := &RAGRetriever{
    KnowledgeBaseDir: "custom_kb_dir",  // 自定义知识库目录
    MaxResults:       5,                // 返回最多 5 个文档
    MinScore:         0.2,              // 最低相关性分数 0.2
}
```

### 知识库文件示例

项目已包含示例知识库文件：
- `knowledge_base/web_scraping_guide.txt` - 网页抓取最佳实践
- `knowledge_base/content_summarization.txt` - 内容摘要生成指南
- `knowledge_base/mcp_tools.txt` - MCP 工具使用说明

### 禁用 RAG

如果不想使用 RAG 功能，可以：
1. 删除或重命名 `knowledge_base` 目录
2. 或者修改 `main.go`，使用 `NewAgent` 而不是 `NewAgentWithRAG`

## 运行示例

**Windows (PowerShell)**
```powershell
set ARK_API_KEY=your_token
# 可选：set ARK_BASE_URL=https://custom-ark-endpoint
go run .
```

**Linux/macOS (Bash)**
```bash
export ARK_API_KEY=your_token
# 可选：export ARK_BASE_URL=https://custom-ark-endpoint
go run .
```

默认任务会抓取 `https://www.sina.com.cn/` 首页，生成摘要并写入 `new.md`。可编辑 `main.go` 中的系统提示或 `agent.Invoke` 的 prompt 以定制行为。

## 部署建议
- 使用 `go build -o mcp-agent` 生成二进制，放置在可信机器上。
- 将 Ark 凭证保存在安全的环境变量或秘密管理系统中。
- 在生产环境使用 systemd/Supervisor 等保障进程存活，并监控 MCP 工具与模型调用日志。
- 若需扩展更多功能，只需新增 MCP 客户端或更换模型参数即可。# go-llm-mcp-rag
