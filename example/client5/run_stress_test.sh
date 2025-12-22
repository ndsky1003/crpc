#!/bin/bash

echo "=== CRPC Stress Test Launcher ==="
echo ""
echo "请选择要运行的测试模式："
echo "1) 标准压力测试 (client5)"
echo "2) 最大性能测试 (client5_max) - 专注于最高QPS"
echo "3) 终极压力测试 (client5_ultimate) - 全面性能测试"
echo "4) 运行所有测试"
echo ""
read -p "请输入选择 (1-4): " choice

case $choice in
    1)
        echo ""
        echo "=== 运行标准压力测试 ==="
        echo ""
        go run client5.go
        ;;
    2)
        echo ""
        echo "=== 运行最大性能测试 ==="
        echo ""
        go run client5_max.go
        ;;
    3)
        echo ""
        echo "=== 运行终极压力测试 ==="
        echo ""
        go run client5_ultimate.go
        ;;
    4)
        echo ""
        echo "=== 运行所有测试 ==="
        echo ""
        echo "1/3 - 标准压力测试"
        go run client5.go

        echo ""
        echo "等待5秒后运行下一个测试..."
        sleep 5

        echo ""
        echo "2/3 - 最大性能测试"
        go run client5_max.go

        echo ""
        echo "等待5秒后运行下一个测试..."
        sleep 5

        echo ""
        echo "3/3 - 终极压力测试"
        go run client5_ultimate.go
        ;;
    *)
        echo "无效选择"
        exit 1
        ;;
esac

echo ""
echo "测试完成！"