# ai-agent

基于 [CloudWeGo Eino](https://github.com/cloudwego/eino) 的企业级 AI Agent 封装。项目把产品协议、Agent 生命周期、模型与运行时、存储、长期记忆和可观测性解耦，企业可以通过独立 Server 或 Go SDK 快速接入。

## 主要特性

- **统一 Agent 契约**：稳定的 `AgentSpec`、`Run`、`RunEvent`、Checkpoint 和 Interrupt/Resume 模型。
- **多运行时**：内置 Eino `ChatModelAgent` 和 `DeepAgent`，通过 Runtime Registry 扩展新的 Agent 策略。
- **版本与发布**：Agent 版本不可变；每个 Run 固定到发布时的版本，支持灰度、回滚和可复现 Resume。
- **多模型接入**：原生支持 OpenAI、Claude、Gemini，并支持 DeepSeek、豆包、Kimi、Qwen、GLM、MiniMax 等主流模型，密钥通过 `SecretResolver` 获取。
- **工具调用与审批**：工具参数校验、风险级别、Interrupt/Resume 和高风险工具人工审批。
- **可插拔存储**：`spi/storage.Storage` 是稳定接口；内置内存和 GORM 存储，支持 MySQL、PostgreSQL，以及通过自定义 GORM Dialector 接入达梦等数据库。
- **长期记忆**：默认关闭，支持 `disabled`、`read_only`、`write_only`、`read_write`；内置租户隔离和可替换 Embedding/Retriever。
- **Checkpoint**：保存 Eino 框架和 Codec 版本，升级时可检测不兼容状态。
- **REST/SSE 网关**：基于 Hertz 提供 Agent 发布、Run、Resume、Run 查询和事件流接口。
- **Go Client**：`pkg/client` 提供启动 Run、消费 SSE、Resume 和查询 Run 的轻量客户端。
- **可观测性扩展**：通过 `spi/observability.Observer` 接入日志、指标、Tracing 和审计系统。
- **多租户边界**：业务查询以 Tenant、Product、User 和 Agent 作用域隔离。

## 架构概览

```text
产品/业务系统
    |
    +--> REST/SSE 或 Go Client
              |
         agentkit.Engine
              |
       contract + SPI 稳定边界
       |       |        |        |
    Runtime  Storage  Memory  Observer
       |       |        |
     Eino    GORM/    Portable
   Chat/Deep  Custom   Retriever
```

核心边界位于 `contract/` 和 `spi/`，不暴露 Eino、GORM、Hertz 或具体模型供应商类型。框架和基础设施适配器位于 `adapter/`。更多设计说明见 [docs/architecture.md](docs/architecture.md)。

## 快速开始：Server

### 环境要求

- Go 1.26 或更高版本
- 至少一个受支持的模型服务和对应 API Key

### 支持的模型供应商

| `provider` | 接入方式 | 默认 API 地址 | 常用密钥变量 |
| --- | --- | --- | --- |
| `openai` | Eino OpenAI | OpenAI 官方地址 | `OPENAI_API_KEY` |
| `claude` / `anthropic` | Eino Claude 原生 Messages API | Anthropic 官方地址 | `ANTHROPIC_API_KEY` |
| `gemini` / `google` | Eino Gemini 原生 API | Google Gemini 官方地址 | `GEMINI_API_KEY` |
| `deepseek` | OpenAI-compatible | `https://api.deepseek.com` | `DEEPSEEK_API_KEY` |
| `doubao-seed` / `doubao` / `ark` | 火山方舟 OpenAI-compatible | `https://ark.cn-beijing.volces.com/api/v3` | `ARK_API_KEY` |
| `kimi` / `moonshot` | OpenAI-compatible | `https://api.moonshot.cn/v1` | `MOONSHOT_API_KEY` |
| `qwen` / `dashscope` | 百炼 OpenAI-compatible | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_API_KEY` |
| `glm` / `zhipu` | OpenAI-compatible | `https://open.bigmodel.cn/api/paas/v4` | `ZHIPU_API_KEY` |
| `minimax` | OpenAI-compatible | `https://api.minimaxi.com/v1` | `MINIMAX_API_KEY` |
| `openai-compatible` | 自定义兼容服务 | 必须配置 `base_url` | 自定义 |

所有默认地址均可通过 `base_url` 覆盖，以接入企业代理、私有网关或兼容服务。同一个 Server 可以同时配置多个供应商，Agent 通过 `model_ref` 选择模型。

### 启动服务

示例配置位于 [configs/config.example.yaml](configs/config.example.yaml)。默认使用内存存储，适合本地开发；生产环境应配置数据库和版本化迁移。

一个同时配置 Claude、Gemini 和 DeepSeek 的示例：

```yaml
models:
  claude-sonnet:
    provider: claude
    model: claude-sonnet-4-5
    credential_ref: ANTHROPIC_API_KEY
    timeout: 60s

  gemini-flash:
    provider: gemini
    model: gemini-2.5-flash
    credential_ref: GEMINI_API_KEY
    timeout: 60s

  deepseek-chat:
    provider: deepseek
    model: deepseek-chat
    credential_ref: DEEPSEEK_API_KEY
    timeout: 60s
```

发布 Agent 时将 `model_ref` 设置为上面的配置名称，例如 `claude-sonnet`。完整的各厂商示例见 [configs/config.example.yaml](configs/config.example.yaml)。

Linux/macOS：

```bash
export OPENAI_API_KEY="your-api-key"
go run ./cmd/agent-server -config configs/config.example.yaml
```

PowerShell：

```powershell
$env:OPENAI_API_KEY = "your-api-key"
go run ./cmd/agent-server -config configs/config.example.yaml
```

服务默认监听 `http://localhost:8080`：

```bash
curl http://localhost:8080/healthz
```

### 发布 Chat Model Agent

`X-Tenant-ID` 和 `X-Product-ID` 用于租户与产品隔离。`activate: true` 会将该版本设为新 Run 使用的活动版本。

```bash
curl -X POST http://localhost:8080/v1/admin/agents \
  -H 'Content-Type: application/json' \
  -H 'X-Tenant-ID: demo' \
  -H 'X-Product-ID: customer-service' \
  -d '{
    "activate": true,
    "spec": {
      "key": "assistant",
      "name": "Customer Assistant",
      "model_ref": "default",
      "runtime": {"type": "chat_model"},
      "memory": {"mode": "disabled"}
    }
  }'
```

### 通过 SSE 执行 Run

```bash
curl -N -X POST http://localhost:8080/v1/agents/assistant/runs \
  -H 'Accept: text/event-stream' \
  -H 'Content-Type: application/json' \
  -H 'X-Tenant-ID: demo' \
  -H 'X-Product-ID: customer-service' \
  -H 'X-User-ID: user-1' \
  -d '{"message":"请介绍一下你的能力"}'
```

不发送 `Accept: text/event-stream` 时，Server 会返回包含 `run` 和 `events` 的 JSON。

### 切换到 DeepAgent

只需发布新的 Agent 版本，业务调用方无需修改：

```bash
curl -X POST http://localhost:8080/v1/admin/agents \
  -H 'Content-Type: application/json' \
  -H 'X-Tenant-ID: demo' \
  -H 'X-Product-ID: customer-service' \
  -d '{
    "activate": true,
    "spec": {
      "key": "assistant",
      "name": "Customer Assistant",
      "model_ref": "default",
      "runtime": {
        "type": "deep_agent",
        "preset_ref": "enterprise-default"
      },
      "memory": {"mode": "disabled"}
    }
  }'
```

已经创建的 Run 仍使用其固定版本；后续新 Run 才会使用 DeepAgent。

### 开启长期记忆

长期记忆默认关闭。发布 Agent 时启用读写模式：

```json
{
  "memory": {
    "mode": "read_write",
    "require_explicit": true,
    "max_items": 8,
    "max_candidates": 500,
    "ttl": "720h"
  }
}
```

启用 `require_explicit` 时，默认只保存以 `记住：`、`请记住：`、`remember:` 等前缀明确要求记忆的用户内容。

## 快速开始：Go Client

`pkg/client` 是对 REST/SSE 网关的轻量封装，不绑定 Eino 类型：

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "io"

    client "github.com/izhw/ai-agent/pkg/client"
)

func main() {
    c := &client.Client{
        BaseURL: "http://localhost:8080", TenantID: "demo",
        ProductID: "customer-service", UserID: "user-1",
    }
    run, stream, err := c.Start(context.Background(), "assistant", "请介绍一下你的能力", "")
    if err != nil { panic(err) }
    defer stream.Close()
    fmt.Println("run:", run.ID)
    for {
        event, err := stream.Next()
        if errors.Is(err, io.EOF) { break }
        if err != nil { panic(err) }
        fmt.Printf("%s: %s\n", event.Type, event.Content)
    }
}
```

常用方法：

```go
ctx := context.Background()
run, err := c.GetRun(ctx, runID)
stream, err := c.Resume(ctx, runID, map[string]any{"approved": true})
```

`RunStream.Next` 返回 `io.EOF` 表示事件流正常结束；使用完成后应调用 `Close` 释放 HTTP 响应体。

## HTTP API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/healthz` | 健康检查 |
| `POST` | `/v1/admin/agents` | 发布 Agent 版本，可激活 |
| `POST` | `/v1/agents/:agent_key/runs` | 启动 Run，支持 JSON 或 SSE |
| `GET` | `/v1/runs/:run_id` | 查询 Run |
| `GET` | `/v1/runs/:run_id/events` | 查询事件，支持 `after`、`limit` |
| `POST` | `/v1/runs/:run_id/resume` | 使用审批目标恢复 Run |

常用请求头：`X-Tenant-ID`、`X-Product-ID`、`X-User-ID`；上层网关需要鉴权时可使用 `Authorization: Bearer ...`。

## 嵌入式使用与扩展

### 自定义存储

GORM 适配器接收已有的 `*gorm.DB`。MySQL 和 PostgreSQL 可直接使用；达梦等数据库可以注入对应 Dialector：

```go
db, err := gorm.Open(customDialector)
store, err := gormstore.New(db, gormstore.WithDialect(gormstore.DialectDameng))
engine, err := agentkit.New(
    agentkit.WithStorage(store),
    agentkit.WithRuntimes(runtimeRegistry),
)
```

不使用 GORM 时，实现 `spi/storage.Storage` 及其仓储接口即可接入其他数据库或服务。生产环境建议使用版本化迁移；`AutoMigrate` 主要用于开发和测试。

### 嵌入 HTTP Server

```go
engine, err := agentkit.New(
    agentkit.WithStorage(store),
    agentkit.WithRuntimes(runtimeRegistry),
    agentkit.WithMemory(memoryService),
    agentkit.WithObserver(observer),
)
if err != nil { return err }

h := server.Default(server.WithHostPorts(":8080"))
httpapi.New(engine).Register(h)
h.Spin()
```

可扩展接口包括：

- `adapter/model.Factory`：注册企业私有模型 Provider，或覆盖为内部模型网关。
- `spi/runtime.Factory`：增加新的运行时或 Agent 编排策略。
- `spi/storage.Storage`：替换数据库和事务实现。
- `spi/memory.Service`：替换记忆提取、Embedding 和检索实现。
- `spi/tool.Tool`：注册业务工具，并通过风险级别接入审批。
- `spi/secrets.Resolver`：接入企业密钥管理系统。
- `spi/observability.Observer`：接入日志、指标、Tracing 和审计系统。

## 开发与校验

```bash
go test ./...
golangci-lint config verify --config .golangci.yml
golangci-lint run --config .golangci.yml
```

提交代码前请保持 `gofmt` 和项目 lint 检查通过。
