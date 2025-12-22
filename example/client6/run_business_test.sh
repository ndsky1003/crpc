#!/bin/bash

echo "========================================"
echo "CRPC 业务数据压力测试启动器"
echo "========================================"
echo ""
echo "测试说明："
echo "  - 使用真实业务数据结构"
echo "  - 模拟电商系统常见场景"
echo "  - 测试时长：1小时"
echo "  - 每5分钟打印性能报告"
echo ""

# 确保编译通过
echo "编译测试程序..."
go build -o client6_business client6_business.go
if [ $? -ne 0 ]; then
    echo "编译失败！"
    exit 1
fi

echo "编译成功！"
echo ""
echo "启动前检查："
echo "1. 确保 server 端正在运行"
echo "2. 确保 client3 业务服务已注册"
echo "3. 确保有足够的系统资源"
echo ""

read -p "按回车键开始测试..."

echo ""
echo "========================================"
echo "开始 1 小时业务压力测试"
echo "开始时间: $(date)"
echo "========================================"
echo ""

# 运行测试
./client6_business

echo ""
echo "测试结束时间: $(date)"