#!/bin/bash

set -euo pipefail

# CyberStrikeAI 一键部署启动脚本
ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT_DIR"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的消息
info() { echo -e "${BLUE}ℹ️  $1${NC}"; }
success() { echo -e "${GREEN}✅ $1${NC}"; }
warning() { echo -e "${YELLOW}⚠️  $1${NC}"; }
error() { echo -e "${RED}❌ $1${NC}"; }

echo ""
echo "=========================================="
echo "  CyberStrikeAI 一键部署启动脚本"
echo "=========================================="
echo ""

CONFIG_FILE="$ROOT_DIR/config.yaml"
VENV_DIR="$ROOT_DIR/venv"
REQUIREMENTS_FILE="$ROOT_DIR/requirements.txt"
BINARY_NAME="cyberstrike-ai"

# 检查配置文件
if [ ! -f "$CONFIG_FILE" ]; then
    error "配置文件 config.yaml 不存在"
    info "请确保在项目根目录运行此脚本"
    exit 1
fi

# 检查并安装 Python 环境
check_python() {
    if ! command -v python3 >/dev/null 2>&1; then
        error "未找到 python3"
        echo ""
        info "请先安装 Python 3.10 或更高版本："
        echo "  macOS:   brew install python3"
        echo "  Ubuntu:  sudo apt-get install python3 python3-venv"
        echo "  CentOS:  sudo yum install python3 python3-pip"
        exit 1
    fi
    
    PYTHON_VERSION=$(python3 --version 2>&1 | awk '{print $2}')
    PYTHON_MAJOR=$(echo "$PYTHON_VERSION" | cut -d. -f1)
    PYTHON_MINOR=$(echo "$PYTHON_VERSION" | cut -d. -f2)
    
    if [ "$PYTHON_MAJOR" -lt 3 ] || ([ "$PYTHON_MAJOR" -eq 3 ] && [ "$PYTHON_MINOR" -lt 10 ]); then
        error "Python 版本过低: $PYTHON_VERSION (需要 3.10+)"
        exit 1
    fi
    
    success "Python 环境检查通过: $PYTHON_VERSION"
}

# 检查并安装 Go 环境
check_go() {
    if ! command -v go >/dev/null 2>&1; then
        error "未找到 Go"
        echo ""
        info "请先安装 Go 1.21 或更高版本："
        echo "  macOS:   brew install go"
        echo "  Ubuntu:  sudo apt-get install golang-go"
        echo "  CentOS:  sudo yum install golang"
        echo "  或访问:  https://go.dev/dl/"
        exit 1
    fi
    
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    GO_MAJOR=$(echo "$GO_VERSION" | cut -d. -f1)
    GO_MINOR=$(echo "$GO_VERSION" | cut -d. -f2)
    
    if [ "$GO_MAJOR" -lt 1 ] || ([ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 21 ]); then
        error "Go 版本过低: $GO_VERSION (需要 1.21+)"
        exit 1
    fi
    
    success "Go 环境检查通过: $(go version)"
}

# 设置 Python 虚拟环境
setup_python_env() {
    if [ ! -d "$VENV_DIR" ]; then
        info "创建 Python 虚拟环境..."
        python3 -m venv "$VENV_DIR"
        success "虚拟环境创建完成"
    else
        info "Python 虚拟环境已存在"
    fi
    
    info "激活虚拟环境..."
    # shellcheck disable=SC1091
    source "$VENV_DIR/bin/activate"
    
    if [ -f "$REQUIREMENTS_FILE" ]; then
        info "安装/更新 Python 依赖..."
        pip install --quiet --upgrade pip >/dev/null 2>&1 || true
        
        # 尝试安装依赖，捕获错误输出
        PIP_LOG=$(mktemp)
        if pip install -r "$REQUIREMENTS_FILE" >"$PIP_LOG" 2>&1; then
            success "Python 依赖安装完成"
        else
            # 检查是否是 angr 安装失败（需要 Rust）
            if grep -q "angr" "$PIP_LOG" && grep -q "Rust compiler\|can't find Rust" "$PIP_LOG"; then
                warning "angr 安装失败（需要 Rust 编译器）"
                echo ""
                info "angr 是可选依赖，主要用于二进制分析工具"
                info "如果需要使用 angr，请先安装 Rust："
                echo "  macOS:   curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh"
                echo "  Ubuntu:  curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh"
                echo "  或访问:  https://rustup.rs/"
                echo ""
                info "其他依赖已安装，可以继续使用（部分工具可能不可用）"
            else
                warning "部分 Python 依赖安装失败，但可以继续尝试运行"
                warning "如果遇到问题，请检查错误信息并手动安装缺失的依赖"
                # 显示最后几行错误信息
                echo ""
                info "错误详情（最后 10 行）："
                tail -n 10 "$PIP_LOG" | sed 's/^/  /'
                echo ""
            fi
        fi
        rm -f "$PIP_LOG"
    else
        warning "未找到 requirements.txt，跳过 Python 依赖安装"
    fi
}

# 构建 Go 项目
build_go_project() {
    info "下载 Go 依赖..."
    go mod download >/dev/null 2>&1 || {
        error "Go 依赖下载失败"
        exit 1
    }
    
    info "构建项目..."
    if go build -o "$BINARY_NAME" cmd/server/main.go 2>&1; then
        success "项目构建完成: $BINARY_NAME"
    else
        error "项目构建失败"
        exit 1
    fi
}

# 检查是否需要重新构建
need_rebuild() {
    if [ ! -f "$BINARY_NAME" ]; then
        return 0  # 需要构建
    fi
    
    # 检查源代码是否有更新
    if [ "$BINARY_NAME" -ot cmd/server/main.go ] || \
       [ "$BINARY_NAME" -ot go.mod ] || \
       find internal cmd -name "*.go" -newer "$BINARY_NAME" 2>/dev/null | grep -q .; then
        return 0  # 需要重新构建
    fi
    
    return 1  # 不需要构建
}

# 主流程
main() {
    # 环境检查
    info "检查运行环境..."
    check_python
    check_go
    echo ""
    
    # 设置 Python 环境
    info "设置 Python 环境..."
    setup_python_env
    echo ""
    
    # 构建 Go 项目
    if need_rebuild; then
        info "准备构建项目..."
        build_go_project
    else
        success "可执行文件已是最新，跳过构建"
    fi
    echo ""
    
    # 启动服务器
    success "所有准备工作完成！"
    echo ""
    info "启动 CyberStrikeAI 服务器..."
    echo "=========================================="
    echo ""
    
    # 运行服务器
    exec "./$BINARY_NAME"
}

# 执行主流程
main
