#!/bin/bash
# GC分析脚本 - 获取内存profile并分析

echo "========== GC热点分析脚本 =========="
echo ""
echo "步骤1: 获取内存分配profile..."
curl -s http://localhost:6060/debug/pprof/allocs > allocs.prof
echo "✓ 已保存到 allocs.prof"
echo ""

echo "步骤2: 分析内存分配最多的函数..."
echo "======================================"
go tool pprof -top allocs.prof | head -30
echo "======================================"
echo ""

echo "步骤3: 查看crpc相关的内存分配..."
echo "======================================"
go tool pprof -list client.* allocs.prof | grep -A 5 "alloc\|total" | head -50
echo "======================================"
echo ""

echo "步骤4: 生成调用图(可选，需要graphviz)..."
echo "执行: go tool pprof -web allocs.prof"
echo ""

echo "详细分析命令:"
echo "  go tool pprof allocs.prof"
echo "  (pprof) top           # 查看Top分配函数"
echo "  (pprof) list <func>   # 查看具体函数的分配"
echo "  (pprof) web           # 可视化调用图"
