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
	"sort"
	"strings"
	"text/template"
)

// CLI 参数定义
var (
	suffix        string
	file_path     string
	out_file_name string
	out_dir       string
)

func init() {
	flag.StringVar(&suffix, "s", "_crpc_server_gen", "生成文件的后缀")
	flag.StringVar(&file_path, "f", "", "源文件路径")
	flag.StringVar(&out_file_name, "o", "", "输出文件名（可选）")
	flag.StringVar(&out_dir, "out_dir", "", "输出目录（可选）")
}

// ImportInfo 存储导入信息
type ImportInfo struct {
	Alias      string
	Path       string
	PrintAlias bool
}

// MethodInfo 存储方法的元数据
type MethodInfo struct {
	Name string

	// 参数相关
	HasCtx   bool
	HasMeta  bool
	MetaType string // 原始类型字符串，e.g. "*Meta" or "Meta"
	HasReq   bool
	ReqType  string // 原始类型字符串，e.g. "*Req" or "string"

	// 返回值相关
	HasRes  bool
	ResType string // 原始类型字符串
	HasErr  bool
}

// StructInfo 存储结构体及其对应的方法
type StructInfo struct {
	Name    string
	Methods []MethodInfo
}

// TemplateData 模板数据
type TemplateData struct {
	Package string
	Imports map[string]*ImportInfo
	Structs []StructInfo
}

func main() {
	flag.Parse()

	// --- 路径处理逻辑 ---
	if file_path == "" {
		dir, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		filename := os.Getenv("GOFILE")
		if filename == "" {
			return
		}
		file_path = filepath.Join(dir, filename)
	}

	if file_path == "" {
		flag.Usage()
		return
	}

	dir, filename := filepath.Split(file_path)
	if !strings.HasSuffix(filename, ".go") {
		panic("源文件后缀必须是 .go")
	}

	if out_dir == "" {
		out_dir = dir
	}

	filenameBase := filename[:len(filename)-3]
	filename_new := fmt.Sprintf("%s%s.go", filenameBase, suffix)
	if out_file_name != "" {
		filename_new = out_file_name
	}

	out_file_path := filepath.Join(out_dir, filename_new)
	// ----------------------------------------

	// 解析 AST
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, file_path, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("Parse file error: %v", err)
	}

	packageName := node.Name.Name
	structMap := make(map[string][]MethodInfo)

	// 收集文件中的所有 import
	allImports := make(map[string]*ImportInfo)
	ast.Inspect(node, func(n ast.Node) bool {
		if imp, ok := n.(*ast.ImportSpec); ok {
			info := handleImportSpec(imp)
			allImports[info.Alias] = info
		}
		return true
	})

	// 收集代码生成需要的 import
	neededImports := make(map[string]*ImportInfo)
	// 基础依赖
	neededImports["context"] = &ImportInfo{Path: "\"context\"", Alias: "context"}
	neededImports["errors"] = &ImportInfo{Path: "\"errors\"", Alias: "errors"}
	neededImports["coder"] = &ImportInfo{Path: "\"github.com/ndsky1003/crpc/v3/coder\"", Alias: "coder"}

	// 遍历 AST
	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Recv == nil {
			return true
		}

		// 获取接收者类型名称
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

		// 忽略未导出方法和 HandleMsg 自身
		if !fn.Name.IsExported() || fn.Name.Name == "HandleMsg" {
			return true
		}

		// 分析方法签名
		info, isValid := analyzeMethod(fn)
		if isValid {
			structMap[recvType] = append(structMap[recvType], info)

			// 收集依赖的包 (Req, Meta, Res)
			collectImports(info.ReqType, allImports, neededImports)
			collectImports(info.MetaType, allImports, neededImports)
			collectImports(info.ResType, allImports, neededImports)
		}
		return true
	})

	// 排序保证生成稳定性
	var structs []StructInfo
	for name, methods := range structMap {
		structs = append(structs, StructInfo{
			Name:    name,
			Methods: methods,
		})
	}
	sort.Slice(structs, func(i, j int) bool {
		return structs[i].Name < structs[j].Name
	})

	if len(structs) == 0 {
		fmt.Println("// No suitable methods found.")
		return
	}

	// 生成代码
	data := TemplateData{
		Package: packageName,
		Imports: neededImports,
		Structs: structs,
	}
	code := generateCode(data)

	// 写入文件
	if err := os.WriteFile(out_file_path, code, 0644); err != nil {
		log.Fatalf("Write file error: %v", err)
	}

	// 使用 goimports 格式化
	_ = exec.Command("goimports", "-w", out_file_path).Run()
}

func handleImportSpec(imp *ast.ImportSpec) *ImportInfo {
	path := imp.Path.Value
	alias := ""
	printAlias := false

	cleanPath := strings.Trim(path, "\"")
	parts := strings.Split(cleanPath, "/")
	baseName := parts[len(parts)-1]

	if imp.Name != nil {
		alias = imp.Name.Name
		if alias != baseName {
			printAlias = true
		}
	} else {
		alias = baseName
	}
	return &ImportInfo{
		Alias:      alias,
		Path:       path,
		PrintAlias: printAlias,
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

// analyzeMethod 分析函数签名是否符合 RPC 要求
// 支持: (Ctx?, Meta?, Req?) -> (Res?, Err?)
func analyzeMethod(fn *ast.FuncDecl) (MethodInfo, bool) {
	info := MethodInfo{Name: fn.Name.Name}

	// 1. 分析参数 (Inputs)
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
	// 检查 Context
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
		// (Req)
		info.HasReq = true
		info.ReqType = exprToString(args[idx].Type)
	case 2:
		// (Meta, Req)
		info.HasMeta = true
		info.MetaType = exprToString(args[idx].Type)
		info.HasReq = true
		info.ReqType = exprToString(args[idx+1].Type)
	default:
		// 不支持 > 2 个非 Context 参数
		return info, false
	}

	// 2. 分析返回值 (Outputs)
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
		// (Res, error)
		if exprToString(results[1].Type) != "error" {
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

func generateCode(data TemplateData) []byte {
	tpl := `// Code generated by gencrpcserverv3. DO NOT EDIT.
package {{.Package}}

import (
{{- range .Imports}}
	{{if .PrintAlias}}{{.Alias}} {{end}}{{.Path}}
{{- end}}
)

{{- range .Structs}}

func (c *{{.Name}}) HandleMsg(ctx context.Context, method string, metaCoderT coder.T, reqCoderT coder.T, metaBytes, bodyBytes []byte) (any, error) {
	switch method {
	{{- range .Methods}}
	case "{{.Name}}":
		// 1. 准备 Req
		{{- if .HasReq}}
		var req {{TypeClean .ReqType}}
		// 为了支持可选反序列化 (e.g. string/[]byte 或空包)，判空
		if len(bodyBytes) > 0 {
			if err := coder.Unmarshal(reqCoderT, bodyBytes, &req); err != nil {
				return nil, err
			}
		}
		{{- end}}

		// 2. 准备 Meta
		{{- if .HasMeta}}
		var meta {{TypeClean .MetaType}}
		if len(metaBytes) > 0 {
			if err := coder.Unmarshal(metaCoderT, metaBytes, &meta); err != nil {
				return nil, err
			}
		}
		{{- end}}

		// 3. 调用方法
		{{- if .HasRes}}
		res{{if .HasErr}}, err{{end}} := c.{{.Name}}({{MethodArgs .}})
		{{- else}}
		{{if .HasErr}}err := {{end}} c.{{.Name}}({{MethodArgs .}})
		{{- end}}

		// 4. 处理返回值
		{{- if .HasErr}}
		if err != nil {
			return nil, err
		}
		{{- end}}

		{{- if .HasRes}}
		return {{if isPointer .ResType}}res{{else}}&res{{end}}, nil
		{{- else}}
		return nil, nil
		{{- end}}

	{{- end}}
	default:
		return nil, errors.New("unknown method: " + method)
	}
}
{{- end}}
`

	funcMap := template.FuncMap{
		// 去除指针符号，获取底层值类型名称
		// e.g. "*Req" -> "Req", "string" -> "string"
		"TypeClean": func(t string) string {
			return strings.TrimPrefix(t, "*")
		},
		"isPointer": func(t string) bool {
			return strings.HasPrefix(t, "*")
		},
		"MethodArgs": func(m MethodInfo) string {
			var args []string
			if m.HasCtx {
				args = append(args, "ctx")
			}
			if m.HasMeta {
				if strings.HasPrefix(m.MetaType, "*") {
					args = append(args, "&meta")
				} else {
					args = append(args, "meta")
				}
			}
			if m.HasReq {
				if strings.HasPrefix(m.ReqType, "*") {
					args = append(args, "&req")
				} else {
					args = append(args, "req")
				}
			}
			return strings.Join(args, ", ")
		},
	}

	t := template.Must(template.New("server").Funcs(funcMap).Parse(tpl))
	var buf bytes.Buffer
	err := t.Execute(&buf, data)
	if err != nil {
		log.Fatalf("Template execute error: %v", err)
	}

	src, err := format.Source(buf.Bytes())
	if err != nil {
		fmt.Println("--- Generated Code (Format Failed) ---")
		fmt.Println(buf.String())
		fmt.Println("--------------------------------------")
		log.Fatalf("Format error: %v", err)
	}
	return src
}
