package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
)

const VERSION = "v3.6.2"

const (
	Annotation_IsSkip   = "IsSkip"
	Annotation_FuncName = "FuncName"
	Annotation_Service  = "Service"
	Annotation_Module   = "Module"
	Annotation_Client   = "Client"
	Annotation_CallType = "CallType"
)

const (
	CallType_Call = "Call"
	CallType_Go   = "Go"
	CallType_Send = "Send"
)

var (
	filePath      string
	outDir        string
	outFileName   string
	pkgName       string
	clientVarName string
	globalService string
	globalModule  string
)

func main() {
	flag.StringVar(&filePath, "f", "", "源文件路径 (e.g. ./msg_game.go)")
	flag.StringVar(&outDir, "out_dir", "", "输出目录")
	flag.StringVar(&outFileName, "o", "", "输出文件名")
	flag.StringVar(&pkgName, "package", "", "生成的包名 (默认与源文件相同)")
	flag.StringVar(&clientVarName, "client", "DefaultClient", "生成的代码内部使用的全局 Client 变量名")
	flag.StringVar(&globalService, "service", "", "全局默认 Service Name (默认为结构体名)")
	flag.StringVar(&globalModule, "module", "", "全局默认 Module Name (默认为 Service Name)")
	flag.Parse()

	handlePaths()

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("Parse error: %v", err)
	}

	if pkgName == "" {
		pkgName = node.Name.Name
	}

	imports := make(map[string]*ImportInfo)
	ast.Inspect(node, func(n ast.Node) bool {
		if imp, ok := n.(*ast.ImportSpec); ok {
			info := handleImportSpec(imp)
			imports[info.Alias] = info
		}
		return true
	})

	methodMap := make(map[string][]MethodEntry)
	neededImports := make(map[string]*ImportInfo)

	// 基础依赖
	neededImports["context"] = &ImportInfo{Path: "\"context\""}
	neededImports["client"] = &ImportInfo{Path: "\"github.com/ndsky1003/crpc/v3/client\""}

	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Recv == nil {
			return true
		}

		// 获取接收者类型
		recvType := ""
		switch t := fn.Recv.List[0].Type.(type) {
		case *ast.StarExpr:
			if ident, ok := t.X.(*ast.Ident); ok {
				recvType = ident.Name
			}
		case *ast.Ident:
			recvType = t.Name
		}

		if recvType == "" {
			return true
		}

		if !fn.Name.IsExported() || fn.Name.Name == "HandleMsg" {
			return true
		}

		// 分析方法签名
		info, ok := analyzeMethod(fn, recvType)
		if ok {
			entry := MethodEntry{
				StructName: recvType,
				Info:       info,
			}
			methodMap[info.GenName] = append(methodMap[info.GenName], entry)

			// 收集依赖
			collectImports(info.ReqType, imports, neededImports)
			collectImports(info.ResType, imports, neededImports)
			collectImports(info.MetaType, imports, neededImports)
		}
		return true
	})

	var genFuncs []GenFuncInfo
	for name, entries := range methodMap {
		hasStructConflict := len(entries) > 1

		for _, entry := range entries {
			baseFuncName := name
			if hasStructConflict {
				baseFuncName = name + entry.StructName
			}

			for _, callType := range entry.Info.CallTypes {
				finalFuncName := baseFuncName
				if callType != CallType_Call {
					finalFuncName = fmt.Sprintf("%s_%s", baseFuncName, callType)
				}

				genFuncs = append(genFuncs, GenFuncInfo{
					FuncName:   finalFuncName,
					CallType:   callType,
					MethodInfo: entry.Info,
				})
			}
		}
	}

	sort.Slice(genFuncs, func(i, j int) bool {
		return genFuncs[i].FuncName < genFuncs[j].FuncName
	})

	if len(genFuncs) == 0 {
		fmt.Println("// No suitable methods found.")
		return
	}

	data := TemplateData{
		Package: pkgName,
		Imports: neededImports,
		Funcs:   genFuncs,
	}

	generateCode(data)
}

type ImportInfo struct {
	Alias string
	Path  string
}

// MethodInfo 存储方法的元数据
type MethodInfo struct {
	OriginalName string
	GenName      string
	Service      string
	Module       string
	Method       string
	ClientVar    string
	CallTypes    []string

	// 入参标记
	HasCtx   bool
	HasMeta  bool
	HasReq   bool
	MetaType string
	ReqType  string

	// 出参标记
	HasRes  bool
	HasErr  bool
	ResType string
}

type MethodEntry struct {
	StructName string
	Info       MethodInfo
}

type GenFuncInfo struct {
	FuncName string
	CallType string
	MethodInfo
}

// ParamsSignature 生成参数列表字符串
func (m GenFuncInfo) ParamsSignature() string {
	var parts []string
	if m.HasCtx {
		parts = append(parts, "ctx context.Context")
	}
	if m.HasMeta {
		parts = append(parts, fmt.Sprintf("meta %s", m.MetaType))
	}
	if m.HasReq {
		parts = append(parts, fmt.Sprintf("req %s", m.ReqType))
	}
	// 永远追加可变参数 opts
	parts = append(parts, "opts ...*client.Option")
	return strings.Join(parts, ", ")
}

// ReturnSignature 生成返回值列表字符串
func (m GenFuncInfo) ReturnSignature() string {
	switch m.CallType {
	case CallType_Go:
		return "*client.Call"
	case CallType_Send:
		return "error"
	default: // Call
		var parts []string
		if m.HasRes {
			parts = append(parts, m.ResType)
		}
		if m.HasErr {
			parts = append(parts, "error")
		}
		if len(parts) == 0 {
			return ""
		}
		if len(parts) == 1 {
			return parts[0]
		}
		return fmt.Sprintf("(%s, %s)", parts[0], parts[1])
	}
}

type TemplateData struct {
	Package string
	Imports map[string]*ImportInfo
	Funcs   []GenFuncInfo
}

func handlePaths() {
	if filePath == "" {
		dir, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		filename := os.Getenv("GOFILE")
		if filename != "" {
			filePath = filepath.Join(dir, filename)
		}
	}

	if filePath == "" {
		flag.Usage()
		os.Exit(1)
	}

	dir, filename := filepath.Split(filePath)
	if !strings.HasSuffix(filename, ".go") {
		log.Fatal("Source file must be .go file")
	}

	if outDir == "" {
		outDir = dir
	}

	if outFileName == "" {
		base := filename[:len(filename)-3]
		outFileName = fmt.Sprintf("%s_client.go", base)
	}
}

func handleImportSpec(imp *ast.ImportSpec) *ImportInfo {
	path := imp.Path.Value
	alias := ""
	if imp.Name != nil {
		alias = imp.Name.Name
	} else {
		parts := strings.Split(strings.Trim(path, "\""), "/")
		alias = parts[len(parts)-1]
	}
	return &ImportInfo{
		Alias: alias,
		Path:  path,
	}
}

func collectImports(typeStr string, allImports map[string]*ImportInfo, neededImports map[string]*ImportInfo) {
	if typeStr == "" {
		return
	}
	clean := strings.TrimLeft(typeStr, "*[]map")
	parts := strings.Split(clean, ".")
	if len(parts) > 1 {
		pkgAlias := parts[0]
		if info, ok := allImports[pkgAlias]; ok {
			neededImports[pkgAlias] = info
		}
	}
}

var commentRegex = regexp.MustCompile(`^//\s*@crpc:\s*([^\s]+):\s*(.*)`)

func getAnnotations(doc *ast.CommentGroup) map[string]string {
	res := make(map[string]string)
	if doc == nil {
		return res
	}
	for _, comment := range doc.List {
		matches := commentRegex.FindStringSubmatch(comment.Text)
		if len(matches) == 3 {
			key := matches[1]
			val := strings.TrimSpace(matches[2])
			res[key] = val
		}
	}
	return res
}

// analyzeMethod 核心分析逻辑
func analyzeMethod(fn *ast.FuncDecl, structName string) (MethodInfo, bool) {
	svc := structName
	if globalService != "" {
		svc = globalService
	}

	mod := svc
	if globalModule != "" {
		mod = globalModule
	}

	info := MethodInfo{
		OriginalName: fn.Name.Name,
		GenName:      fn.Name.Name,
		Service:      svc,
		Module:       mod,
		Method:       fn.Name.Name,
		ClientVar:    clientVarName,
		CallTypes:    []string{CallType_Call},
	}

	// 1. 处理注解
	annotations := getAnnotations(fn.Doc)
	if val, ok := annotations[Annotation_IsSkip]; ok && (val == "true" || val == "1") {
		return info, false
	}
	if val, ok := annotations[Annotation_FuncName]; ok {
		info.GenName = val
	}
	if val, ok := annotations[Annotation_Service]; ok {
		info.Service = val
	}
	if val, ok := annotations[Annotation_Module]; ok {
		info.Module = val
	}
	if val, ok := annotations[Annotation_Client]; ok {
		info.ClientVar = val
	}
	if val, ok := annotations[Annotation_CallType]; ok {
		parts := strings.Split(val, ",")
		var types []string
		for _, part := range parts {
			t := strings.TrimSpace(part)
			switch t {
			case CallType_Call, CallType_Go, CallType_Send:
				types = append(types, t)
			}
		}
		if len(types) > 0 {
			info.CallTypes = types
		}
	}

	// 2. 分析参数 (Inputs)
	params := fn.Type.Params.List
	var args []*ast.Field
	for _, p := range params {
		if len(p.Names) > 0 {
			for range p.Names {
				args = append(args, p)
			}
		} else {
			args = append(args, p)
		}
	}

	idx := 0
	// 检查第一个参数是否为 Context
	if len(args) > 0 {
		tStr := exprToString(args[0].Type)
		if strings.Contains(tStr, "Context") {
			info.HasCtx = true
			idx++
		}
	}

	remaining := len(args) - idx
	switch remaining {
	case 0:
		// 无 Meta, 无 Req
	case 1:
		// 只有 1 个参数，默认为 Req
		info.HasReq = true
		info.ReqType = exprToString(args[idx].Type)
	case 2:
		// 有 2 个参数，默认为 Meta, Req
		info.HasMeta = true
		info.MetaType = exprToString(args[idx].Type)
		info.HasReq = true
		info.ReqType = exprToString(args[idx+1].Type)
	default:
		// 不支持 > 2 个非 Context 参数
		return info, false
	}

	// 3. 分析返回值 (Outputs)
	// [修复] 增加判空保护，防止 fn.Type.Results 为 nil 时 panic
	var results []*ast.Field
	if fn.Type.Results != nil {
		results = fn.Type.Results.List
	}
	resLen := len(results)

	switch resLen {
	case 0:
		// 无返回值
	case 1:
		tStr := exprToString(results[0].Type)
		if tStr == "error" {
			info.HasErr = true
		} else {
			info.HasRes = true
			info.ResType = tStr
		}
	case 2:
		// 强制要求 (Res, error)
		lastType := exprToString(results[1].Type)
		if lastType != "error" {
			return info, false
		}
		info.HasRes = true
		info.ResType = exprToString(results[0].Type)
		info.HasErr = true
	default:
		return info, false
	}

	return info, true
}

func exprToString(expr ast.Expr) string {
	var buf bytes.Buffer
	format.Node(&buf, token.NewFileSet(), expr)
	return buf.String()
}

func generateCode(data TemplateData) {
	tpl := `// Code generated by gencrpcclientv3. DO NOT EDIT.
package {{.Package}}

import (
{{- range .Imports}}
	{{if .Alias}}{{.Alias}} {{end}}{{.Path}}
{{- end}}
)

{{- range .Funcs}}

// {{.FuncName}} invokes {{.Service}}.{{.Module}}.{{.Method}}
func {{.FuncName}}({{.ParamsSignature}}) {{.ReturnSignature}} {
	// 1. 构建动态参数
	var argsList []any
	_ = argsList // 防止未使用报错

	// Meta 处理 (放入 opts)
	{{- if .HasMeta}}
	opts = append(opts, client.Options().SetMeta(meta))
	{{- end}}

	// Req 处理
	{{- if .HasReq}}
	var reqBody any = req
	{{- else}}
	var reqBody any = nil
	{{- end}}

	// Ctx 处理
	{{- if not .HasCtx}}
	ctx := context.Background()
	{{- end}}

	// 2. 根据 CallType 分发调用
	{{- if eq .CallType "Go"}}
		// Go 模式：异步调用，返回 Call 对象
		{{- if .HasRes}}
		var res {{ResTypeClean .}}
		return {{.ClientVar}}.Go(ctx, "{{.Service}}", "{{.Module}}.{{.Method}}", reqBody, &res, opts...)
		{{- else}}
		return {{.ClientVar}}.Go(ctx, "{{.Service}}", "{{.Module}}.{{.Method}}", reqBody, nil, opts...)
		{{- end}}

	{{- else if eq .CallType "Send"}}
		// Send 模式：单向发送
		return {{.ClientVar}}.Send(ctx, "{{.Service}}", "{{.Module}}.{{.Method}}", reqBody, opts...)

	{{- else}}
		// Call 模式：同步等待
		{{- if .HasRes}}
		var res {{ResTypeClean .}}
		var resPtr any = &res
		{{- else}}
		var resPtr any = nil
		{{- end}}

		// [修复] 根据是否需要 Error 返回值，动态生成接收变量
		// 如果只有返回值没有 error，使用 _ 忽略错误，防止编译报错 "err declared but not used"
		{{if .HasErr}}err{{else}}_{{end}} := {{.ClientVar}}.Call(ctx, "{{.Service}}", "{{.Module}}.{{.Method}}", reqBody, resPtr, opts...)

		// 3. 处理返回值
		{{- if and .HasRes .HasErr}}
			return {{if isPointer .ResType}}&res{{else}}res{{end}}, err
		{{- else if .HasErr}}
			return err
		{{- else if .HasRes}}
			// 只有返回值，忽略错误
			return {{if isPointer .ResType}}&res{{else}}res{{end}}
		{{- else}}
			// 无返回值
			return
		{{- end}}

	{{- end}}
}
{{- end}}
`

	funcMap := template.FuncMap{
		"ResTypeClean": func(m GenFuncInfo) string {
			return strings.TrimPrefix(m.ResType, "*")
		},
		"isPointer": func(t string) bool {
			return strings.HasPrefix(t, "*")
		},
	}

	t := template.Must(template.New("client").Funcs(funcMap).Parse(tpl))
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		log.Fatalf("Template execute error: %v", err)
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Fatalf("Mkdir error: %v", err)
	}

	outPath := filepath.Join(outDir, outFileName)
	if err := os.WriteFile(outPath, buf.Bytes(), 0644); err != nil {
		log.Fatalf("Write file error: %v", err)
	}

	_ = exec.Command("goimports", "-w", outPath).Run()
	_ = exec.Command("gofmt", "-w", outPath).Run()
}
