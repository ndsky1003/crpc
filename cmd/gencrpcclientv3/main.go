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

const VERSION = "v3.5.3"

const (
	Annotation_IsSkip   = "IsSkip"
	Annotation_FuncName = "FuncName"
	Annotation_Service  = "Service"
	Annotation_Module   = "Module"
	Annotation_Client   = "Client"
	Annotation_CallType = "CallType"
)

const (
	CallType_Call      = "Call"
	CallType_Broadcast = "Broadcast"
	CallType_Go        = "Go"
	CallType_Send      = "Send"
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

	neededImports["context"] = &ImportInfo{Path: "\"context\""}
	neededImports["client"] = &ImportInfo{Path: "\"github.com/ndsky1003/crpc/v3/client\""}

	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Recv == nil {
			return true
		}

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

		info, ok := analyzeMethod(fn, recvType)
		if ok {
			entry := MethodEntry{
				StructName: recvType,
				Info:       info,
			}
			methodMap[info.GenName] = append(methodMap[info.GenName], entry)

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

type MethodInfo struct {
	OriginalName string
	GenName      string
	Service      string
	Module       string
	Method       string
	ClientVar    string
	CallTypes    []string

	HasCtx    bool
	MetaType  string
	ReqType   string
	ResType   string
	HasReturn bool
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

// ReturnSignature 根据 CallType 决定返回值
// Broadcast 和 Send 现在只返回 error，因为结果通过回调处理或不需要
func (m GenFuncInfo) ReturnSignature() string {
	switch m.CallType {
	case CallType_Go:
		return "*client.Call"
	case CallType_Send:
		return "error"
	case CallType_Broadcast:
		return "error"
	default: // Call
		if m.HasReturn {
			return fmt.Sprintf("(%s, error)", m.ResType)
		}
		return "error"
	}
}

type TemplateData struct {
	Package string
	Imports map[string]*ImportInfo
	Funcs   []GenFuncInfo
}

func toGenFunc(entry MethodEntry, finalName string) GenFuncInfo {
	return GenFuncInfo{
		FuncName:   finalName,
		MethodInfo: entry.Info,
	}
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
			case CallType_Call, CallType_Broadcast, CallType_Go, CallType_Send:
				types = append(types, t)
			}
		}
		if len(types) > 0 {
			info.CallTypes = types
		}
	}

	params := fn.Type.Params.List
	args := make([]*ast.Field, 0)
	for _, p := range params {
		if len(p.Names) > 0 {
			for range p.Names {
				args = append(args, p)
			}
		} else {
			args = append(args, p)
		}
	}

	if len(args) == 0 {
		return info, false
	}

	firstType := exprToString(args[0].Type)
	if strings.Contains(firstType, "Context") {
		info.HasCtx = true
		args = args[1:]
	}

	if len(args) == 2 {
		info.MetaType = exprToString(args[0].Type)
		info.ReqType = exprToString(args[1].Type)
	} else if len(args) == 1 {
		info.ReqType = exprToString(args[0].Type)
	} else {
		return info, false
	}

	results := fn.Type.Results.List
	if len(results) == 0 {
		return info, false
	}
	lastRes := results[len(results)-1]
	if exprToString(lastRes.Type) != "error" {
		return info, false
	}

	if len(results) == 2 {
		info.HasReturn = true
		info.ResType = exprToString(results[0].Type)
	} else if len(results) > 2 {
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
func {{.FuncName}}(ctx context.Context, {{if .MetaType}}meta {{.MetaType}}, {{end}}req {{.ReqType}}, opts ...*client.Option) {{.ReturnSignature}} {
	{{- if or (eq .CallType "Call") (eq .CallType "Go") }}
	{{- if .HasReturn}}
	var res {{ResTypeClean .}}
	{{- end}}
	{{- end}}
	
	{{- if .MetaType}}
	opts = append(opts, client.Options().SetMeta(meta))
	{{- end}}

	{{- if eq .CallType "Go"}}
	return {{.ClientVar}}.Go(ctx, "{{.Service}}", "{{.Module}}.{{.Method}}", req, {{if .HasReturn}}&res{{else}}nil{{end}}, opts...)
	
	{{- else if eq .CallType "Send"}}
	return {{.ClientVar}}.Send(ctx, "{{.Service}}", "{{.Module}}.{{.Method}}", req, opts...)
	
	{{- else if eq .CallType "Broadcast"}}
	return {{.ClientVar}}.Broadcast(ctx, "{{.Service}}", "{{.Module}}.{{.Method}}", req, opts...)

	{{- else}}
	err := {{.ClientVar}}.Call(ctx, "{{.Service}}", "{{.Module}}.{{.Method}}", req, {{if .HasReturn}}&res{{else}}nil{{end}}, opts...)
	{{- if .HasReturn}}
	return {{if isPointer .ResType}}&res{{else}}res{{end}}, err
	{{- else}}
	return err
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

	exec.Command("goimports", "-w", outPath).Run()
	exec.Command("gofmt", "-w", outPath).Run()

}
