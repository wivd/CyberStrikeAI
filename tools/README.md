# 工具配置文件说明

## 概述

每个工具现在都有独立的配置文件，存放在 `tools/` 目录下。这种方式使得工具配置更加清晰、易于维护和管理。

## 配置文件格式

每个工具配置文件是一个 YAML 文件，包含以下字段：

### 必需字段

- `name`: 工具名称（唯一标识符）
- `command`: 要执行的命令
- `enabled`: 是否启用（true/false）

### 可选字段

- `args`: 固定参数列表（数组）
- `short_description`: 简短描述（一句话说明工具用途，用于工具列表，减少token消耗）
- `description`: 工具详细描述（支持多行文本，用于工具文档和详细说明）
- `parameters`: 参数定义列表

## 工具描述

### 简短描述 (`short_description`)

- **用途**：用于工具列表，减少发送给大模型的token消耗
- **要求**：一句话（20-50字）说明工具的核心用途
- **示例**：`"网络扫描工具，用于发现网络主机、开放端口和服务"`

### 详细描述 (`description`)

支持多行文本，应该包含：

1. **工具功能说明**：工具的主要功能
2. **使用场景**：什么情况下使用这个工具
3. **注意事项**：使用时的注意事项和警告
4. **示例**：使用示例（可选）

**重要说明**：
- 工具列表发送给大模型时，使用 `short_description`（如果存在）
- 如果没有 `short_description`，系统会自动从 `description` 中提取第一行或前100个字符
- 详细描述可以通过 MCP 的 `resources/read` 接口获取（URI: `tool://tool_name`）

这样可以大幅减少token消耗，特别是当工具数量很多时（如100个工具）。

## 参数定义

每个参数可以包含以下字段：

- `name`: 参数名称
- `type`: 参数类型（string, int, bool, array）
- `description`: 参数详细描述（支持多行）
- `required`: 是否必需（true/false）
- `default`: 默认值
- `flag`: 命令行标志（如 "-u", "--url"）
- `position`: 位置参数的位置（整数）
- `format`: 参数格式（"flag", "positional", "combined", "template"）
- `template`: 模板字符串（用于 format="template"）
- `options`: 可选值列表（用于枚举类型）

### 参数描述要求

参数描述应该包含：

1. **参数用途**：这个参数是做什么的
2. **格式要求**：参数值的格式要求
3. **示例值**：具体的示例值
4. **注意事项**：使用时需要注意的事项

## 示例

参考 `tools/` 目录下的现有工具配置文件：

- `nmap.yaml`: 网络扫描工具
- `sqlmap.yaml`: SQL注入检测工具
- `nikto.yaml`: Web服务器扫描工具
- `dirb.yaml`: Web目录扫描工具
- `exec.yaml`: 系统命令执行工具

## 添加新工具

要添加新工具，只需在 `tools/` 目录下创建一个新的 YAML 文件，例如 `my_tool.yaml`：

```yaml
name: "my_tool"
command: "my-command"
enabled: true
short_description: "一句话说明工具用途"  # 简短描述（推荐）
description: |
  工具详细描述...
  
  **功能：**
  - 功能1
  - 功能2
  
  **使用场景：**
  - 场景1
  - 场景2

parameters:
  - name: "param1"
    type: "string"
    description: |
      参数详细描述...
      
      **示例值：**
      - "value1"
      - "value2"
    required: true
    flag: "-p"
    format: "flag"
```

保存文件后，重启服务即可自动加载新工具。

## 禁用工具

要禁用某个工具，只需将配置文件中的 `enabled` 字段设置为 `false`，或者直接删除/重命名配置文件。

