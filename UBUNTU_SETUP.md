# Ubuntu 22.04 启动指南

本文档说明如何在 Ubuntu 22.04 上启动 `llm-mcp-rag` 项目。

## 前置要求

1. **Go 1.21+**
2. **uv**（Python 包管理器，用于运行 `mcp-server-fetch`）
3. **Node.js 和 npm**（用于运行 `@modelcontextprotocol/server-filesystem`）
4. **火山方舟 API Key**（`ARK_API_KEY`）

## 安装步骤

### 1. 安装 Go

```bash
# 检查是否已安装 Go
go version

# 如果未安装或版本低于 1.21，执行以下命令安装
sudo apt update
sudo apt install -y golang-go

# 或者安装最新版本（推荐）
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# 验证安装
go version
```

### 2. 安装 uv（Python 包管理器）

```bash
# 使用官方安装脚本
curl -LsSf https://astral.sh/uv/install.sh | sh

# 添加到 PATH（如果未自动添加）
echo 'export PATH="$HOME/.cargo/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc

# 验证安装
uv --version
```

### 3. 安装 Node.js 和 npm

```bash
# 使用 NodeSource 仓库安装 Node.js 20.x（LTS）
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt install -y nodejs

# 验证安装
node --version
npm --version
```

### 4. 配置环境变量

```bash
# 设置火山方舟 API Key（必需）
export ARK_API_KEY="your_api_key_here"

# 可选：自定义网关地址
export ARK_BASE_URL="https://custom-ark-endpoint"

# 将环境变量持久化到 ~/.bashrc（可选）
echo 'export ARK_API_KEY="your_api_key_here"' >> ~/.bashrc
# echo 'export ARK_BASE_URL="https://custom-ark-endpoint"' >> ~/.bashrc
source ~/.bashrc
```

### 5. 下载项目依赖

```bash
cd /home/lucien/goproject/llm-mcp-rag
go mod download
```

### 6. 运行项目

```bash
# 确保已设置 ARK_API_KEY
echo $ARK_API_KEY

# 运行项目
go run .

# 或者先编译再运行
go build -o mcp-agent
./mcp-agent
```

## 快速启动脚本

你也可以使用提供的 `setup_ubuntu.sh` 脚本自动完成大部分安装步骤（需要手动设置 API Key）。

## 故障排查

### 问题 1: `uvx: command not found`
- 确保 `uv` 已正确安装并在 PATH 中
- 运行 `which uv` 检查路径
- 重新加载 shell：`source ~/.bashrc`

### 问题 2: `npx: command not found`
- 确保 Node.js 和 npm 已正确安装
- 运行 `which node` 和 `which npm` 检查
- 如果未找到，重新安装 Node.js

### 问题 3: `ARK_API_KEY` 未设置
- 检查环境变量：`echo $ARK_API_KEY`
- 确保在运行前已设置：`export ARK_API_KEY="your_key"`

### 问题 4: Go 版本过低
- 检查版本：`go version`
- 如果低于 1.21，按照步骤 1 升级 Go

### 问题 5: MCP 工具启动失败
- 检查网络连接（需要下载 MCP 服务器）
- 确保 `uv` 和 `npx` 可以正常访问互联网
- 查看错误日志中的具体错误信息

## 验证安装

运行以下命令验证所有依赖已正确安装：

```bash
echo "Go version: $(go version)"
echo "uv version: $(uv --version)"
echo "Node version: $(node --version)"
echo "npm version: $(npm --version)"
echo "ARK_API_KEY: ${ARK_API_KEY:+已设置}"
```

## 项目说明

项目默认会：
1. 使用 `mcp-server-fetch` 抓取 `https://news.ycombinator.com` 首页
2. 使用 Doubao 模型总结内容
3. 使用 `@modelcontextprotocol/server-filesystem` 将结果写入 `new.md`

你可以修改 `main.go` 中的系统提示或 `agent.Invoke` 的 prompt 来定制行为。

