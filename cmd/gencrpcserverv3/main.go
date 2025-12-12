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

type ImportInfo struct {
	Alias      string
	Path       string
	PrintAlias bool
}

type MethodInfo struct {
	Name     string
	HasCtx   bool
	HasMeta  bool
	MetaType string
	HasReq   bool
	ReqType  string
	HasRes   bool
	ResType  string
	HasErr   bool
}

type StructInfo struct {
	Name    string
	Methods []MethodInfo
}

type TemplateData struct {
	Package string
	Imports map[string]*ImportInfo
	Structs []StructInfo
}

func main() {
	flag.Parse()

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

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, file_path, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("Parse file error: %v", err)
	}

	packageName := node.Name.Name
	structMap := make(map[string][]MethodInfo)

	allImports := make(map[string]*ImportInfo)
	ast.Inspect(node, func(n ast.Node) bool {
		if imp, ok := n.(*ast.ImportSpec); ok {
			info := handleImportSpec(imp)
			allImports[info.Alias] = info
		}
		return true
	})

	neededImports := make(map[string]*ImportInfo)
	neededImports["context"] = &ImportInfo{Path: "\"context\"", Alias: "context"}
	neededImports["errors"] = &ImportInfo{Path: "\"errors\"", Alias: "errors"}
	neededImports["coder"] = &ImportInfo{Path: "\"github.com/ndsky1003/crpc/v3/coder\"", Alias: "coder"}

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

		info, isValid := analyzeMethod(fn)
		if isValid {
			structMap[recvType] = append(structMap[recvType], info)
			collectImports(info.ReqType, allImports, neededImports)
			collectImports(info.MetaType, allImports, neededImports)
			collectImports(info.ResType, allImports, neededImports)
		}
		return true
	})

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

	data := TemplateData{
		Package: packageName,
		Imports: neededImports,
		Structs: structs,
	}
	code := generateCode(data)

	if err := os.WriteFile(out_file_path, code, 0644); err != nil {
		log.Fatalf("Write file error: %v", err)
	}

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

func analyzeMethod(fn *ast.FuncDecl) (MethodInfo, bool) {
	info := MethodInfo{Name: fn.Name.Name}

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
	case 1:
		info.HasReq = true
		info.ReqType = exprToString(args[idx].Type)
	case 2:
		info.HasMeta = true
		info.MetaType = exprToString(args[idx].Type)
		info.HasReq = true
		info.ReqType = exprToString(args[idx+1].Type)
	default:
		return info, false
	}

	var results []*ast.Field
	if fn.Type.Results != nil {
		results = fn.Type.Results.List
	}
	resLen := len(results)

	switch resLen {
	case 0:
	case 1:
		tStr := exprToString(results[0].Type)
		if tStr == "error" {
			info.HasErr = true
		} else {
			info.HasRes = true
			info.ResType = tStr
		}
	case 2:
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

// [修改] 修复了模板逻辑：无论方法参数是指针还是值，req 变量本身都是值类型，
// 因此从 bodyData 获取数据时，必须始终赋值为值（如果是指针则解引用）。
func generateCode(data TemplateData) []byte {
	tpl := `// Code generated by gencrpcserverv3. DO NOT EDIT.
package {{.Package}}

import (
{{- range .Imports}}
	{{if .PrintAlias}}{{.Alias}} {{end}}{{.Path}}
{{- end}}
)

{{- range .Structs}}

func (c *{{.Name}}) HandleMsg(ctx context.Context, method string, metaCoderT coder.T, reqCoderT coder.T, metaData, bodyData any) (any, error) {
	switch method {
	{{- range .Methods}}
	case "{{.Name}}":
		// 1. 准备 Req
		{{- if .HasReq}}
		var req {{TypeClean .ReqType}}
		
		if b, ok := bodyData.([]byte); ok {
			// 远程调用 (Bytes)
			if len(b) > 0 {
				if err := coder.Unmarshal(reqCoderT, b, &req); err != nil {
					return nil, err
				}
			}
		} else if bodyData != nil {
			// 本地调用 (Object)
			if v, ok := bodyData.(*{{TypeClean .ReqType}}); ok {
				if v != nil {
					req = *v
				}
			} else if v, ok := bodyData.({{TypeClean .ReqType}}); ok {
				req = v
			} else {
				return nil, errors.New("local call type mismatch for {{.Name}} arg: req")
			}
		}
		{{- end}}

		// 2. 准备 Meta
		{{- if .HasMeta}}
		var meta {{TypeClean .MetaType}}
		if b, ok := metaData.([]byte); ok {
			if len(b) > 0 {
				if err := coder.Unmarshal(metaCoderT, b, &meta); err != nil {
					return nil, err
				}
			}
		} else if metaData != nil {
			if v, ok := metaData.(*{{TypeClean .MetaType}}); ok {
				if v != nil {
					meta = *v
				}
			} else if v, ok := metaData.({{TypeClean .MetaType}}); ok {
				meta = v
			} else {
				return nil, errors.New("local call type mismatch for {{.Name}} arg: meta")
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
