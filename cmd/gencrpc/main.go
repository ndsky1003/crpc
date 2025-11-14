package main

import (
	_ "embed"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

const VERSION = "v2.0.17"

const (
	Annotation_IsSkip         = "IsSkip"   // 跳过这个方法自动生成
	Annotation_SubAsync       = "SubAsync" // 异步方法的后缀
	Annotation_Content        = "Content"  //完全替代内容实现，不包含函数签名
	Annotation_FuncName       = "FuncName"
	Annotation_Client         = "Client"
	Annotation_Server         = "Server"
	Annotation_Module         = "Module"
	Annotation_ReqAppend      = "ReqAppend"
	Annotation_TypeElemInit   = "TypeElemInit" // var %v int ,var %v = MyType{}, %v = MyMap{} 注意占位符,一般都是自定义类型需要，eg: type MyMap map[string]int 这种
	Annotation_CallOptionName = "CallOptionName"
	Annotation_SetOption      = "SetOption"
)

var (
	file_path     string
	out_dir       string
	out_file_name string
	suffix        string
)
var out_file_path string

// 格式化 Go 文件
func formatGoFile(filePath string) error {
	cmd := exec.Command("goimports", "-w", filePath)
	return cmd.Run()
}

func main() {
	useage_flag := flag.Bool("useage", false, "显示使用规则")
	version_flag := flag.Bool("version", false, "显示版本信息")
	v_flag := flag.Bool("v", false, "显示版本信息")
	annotation_flag := flag.Bool("annotation", false, "显示标注信息")
	a_flag := flag.Bool("a", false, "显示标注信息")
	flag.StringVar(&file_path, "f", "", "需要解析的文件")
	flag.StringVar(&out_dir, "out_dir", "", "输出的新文件目录")
	flag.StringVar(&out_file_name, "out_file_name", "", "输出的新文件名")
	flag.StringVar(&suffix, "sub", "_crpc_gen", "输出的新文件的后缀")
	flag.StringVar(&data.Imports, "import", "", "额外需要导入的包，多个包用逗号分隔,别名用冒号分开。eg: --import=jj:encoding/json,tt:time")
	flag.StringVar(&data.PackageName, "package", "main", "生成代码时使用的包名")
	flag.BoolVar(&data.IsSync, "sync", true, "是否生成同步方法")
	flag.BoolVar(&data.IsAsync, "async", false, "是否生成异步方法")
	flag.StringVar(&data.SubAsync, "sub_async", "Async", "异步方法的后缀")
	flag.StringVar(&data.Client, "client", "crpc_client", "生成客户端代码时使用的变量名")
	flag.StringVar(&data.Server, "server", "crpc_server_name", "调用哪个服务")
	flag.StringVar(&data.Module, "module", "crpc", "生成代码时使用的模块名")
	flag.StringVar(&data.Req_Append, "req_append", "opts:...crpc.Option", "额外参数，多个参数用逗号分隔,冒号分割变量与类型,注解支持空格,命令行不支持空格,所以用冒号。eg: --append_req=tt:time,opts:...crpc.Option")
	flag.StringVar(&data.Call_Option_Name, "call_option_name", "", "就是调用call的时候的那个属性名字")
	flag.Parse()
	if *version_flag || *v_flag {
		fmt.Println(VERSION)
		return
	}

	if *useage_flag {
		fmt.Println("使用规则:")
		fmt.Println("go文件://go:generate gencrpc --import=jingtw/comm/crpc,jingtw/comm/gamename --out_dir=../comm/crpc/db/ --package=db --req_append=opts:...*crpc.Option --client=crpc.GameClient --server=string(gamename.DB)")
		fmt.Println("终端:gencrpc --import=jingtw/comm/crpc,jingtw/comm/gamename --out_dir=../comm/crpc/db/ --package=db --req_append=opts ...crpc.Option --client=crpc.GameClient --server=string(gamename.DB)")
		return
	}

	if *annotation_flag || *a_flag {
		fmt.Println("标注信息:")
		fmt.Printf("  %s: 跳过这个方法自动生成\n", Annotation_IsSkip)
		fmt.Printf("  %s: 异步方法的后缀\n", Annotation_SubAsync)
		fmt.Printf("  %s: 完全替代内容实现，不包含函数签名\n", Annotation_Content)
		fmt.Printf("  %s: 函数名，可以被覆盖\n", Annotation_FuncName)
		fmt.Printf("  %s: 生成客户端代码时使用的变量名\n", Annotation_Client)
		fmt.Printf("  %s: 调	用哪个服务\n", Annotation_Server)
		fmt.Printf("  %s: 生成代码时使用的模块名\n", Annotation_Module)
		fmt.Printf("  %s: 额外参数，多个参数用逗号分隔。eg: --append_req=tt time,opts ...crpc.Option\n", Annotation_ReqAppend)
		fmt.Printf("  %s: var %%v int ,var %%v = MyType{}, %%v = MyMap{} 注意占位符,一般都是自定义类型需要，eg: type MyMap map[string]int 这种\n", Annotation_TypeElemInit)
		fmt.Printf("  %s: 就是调用call的时候的那个属性名字\n", Annotation_CallOptionName)
		s := `  // @crpc: FuncName: func111
  // @crpc: Client: crpc_client_ano
  // @crpc: SubAsync: Async
  // @crpc: Server: server2
  // @crpc: Module:mod3
  // @crpc: ReqAppend: opts ...crpc.Option
  // @crpc: IsSkip: 2
  // @crpc: CallOptionName: opt
  // @crpc: TypeElemInit:var %v = MyMap{}
  // @crpc: TypeElemInit: %v = Myerror{}
  // @crpc: SetOption: SetCoderT(coder.Msgp)
  // @crpc: SetOption: SetCoderT(coder.Msgp)
`
		fmt.Println(s)
		return
	}

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
	dir, filename := filepath.Split(file_path)
	if !strings.HasSuffix(filename, ".go") {
		panic("源文件后缀必须是 .go")
	}
	if out_dir == "" {
		out_dir = dir
	}
	filename = filename[:len(filename)-3]
	filename_new := fmt.Sprintf("%s%s.go", filename, suffix)
	if out_file_name != "" {
		filename_new = out_file_name
	}

	out_file_path = filepath.Join(out_dir, filename_new)

	// 解析 Go 文件
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, file_path, nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}

	// 获取包名
	if data.PackageName == "" {
		data.PackageName = node.Name.Name
	}

	// 遍历 AST
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.ImportSpec:
			if importor := handleImportSpec(n); importor != nil {
				data.Importor_all[importor.Name] = importor
			}
		case *ast.FuncDecl:
			if data.IsSync {
				if func_desc := handleFuncDecl(n, false); func_desc != nil {
					data.Funcs = append(data.Funcs, func_desc)
				}
			}
			if data.IsAsync {
				if func_desc := handleFuncDecl(n, true); func_desc != nil {
					data.Funcs = append(data.Funcs, func_desc)
				}
			}
		}
		return true
	})
	data.FixImportorNeedList()
	tmpl, err := template.New("tmpl").Parse(tmpl)
	if err != nil {
		panic(err)
	}

	outFile, err := os.OpenFile(out_file_path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0644)
	if err != nil {
		fmt.Println("无法创建输出文件:", err)
		return
	}
	defer outFile.Close()

	if err := tmpl.Execute(outFile, data); err != nil {
		panic(err)
	}

	if err := formatGoFile(out_file_path); err != nil {
		panic(err)
	}

}

type import_value struct {
	Name     string
	Path     string
	IsIndent bool
}

func (this *import_value) String() string {
	return fmt.Sprintf("Name:%s,Path:%s,IsIndent:%v", this.Name, this.Path, this.IsIndent)
}

type Data struct {
	Importor_all     map[string]*import_value
	Importor_need    map[string]*import_value
	Importor_extra   []*import_value
	Imports          string
	PackageName      string
	IsSync           bool //是否生成同步方法
	IsAsync          bool //是否生成异步方法
	SubAsync         string
	Client           string
	Server           string
	Module           string
	Req_Append       string
	Call_Option_Name string
	Funcs            []*func_decl
}

func (this *Data) FixImportorNeedList() {
	for _, f := range this.Funcs {
		for _, in := range f.In {
			if in.ImportPre == "" {
				continue
			}
			if iv, ok := this.Importor_all[in.ImportPre]; ok {
				this.Importor_need[in.ImportPre] = iv
			}
		}
		for _, out := range f.Out {
			if out.ImportPre == "" {
				continue
			}
			if iv, ok := this.Importor_all[out.ImportPre]; ok {
				this.Importor_need[out.ImportPre] = iv
			}
		}
	}
	if imports := data.Imports; imports != "" {
		parts := strings.Split(imports, ",")
		for _, part := range parts {
			subparts := strings.SplitN(part, ":", 2)
			if len(subparts) == 2 {
				name := subparts[0]
				path := subparts[1]
				this.Importor_extra = append(this.Importor_extra, &import_value{
					Name:     name,
					Path:     path,
					IsIndent: name != "",
				})
			} else {
				this.Importor_extra = append(this.Importor_extra, &import_value{
					Name:     "",
					Path:     part,
					IsIndent: false,
				})
			}
		}
	}
}

var data = &Data{
	Importor_all:  map[string]*import_value{},
	Importor_need: map[string]*import_value{},
}

func handleImportSpec(importSpec *ast.ImportSpec) *import_value {
	path := strings.Trim(importSpec.Path.Value, `"`)
	name := filepath.Base(path)
	isIndent := false
	if importSpec.Name != nil {
		name = importSpec.Name.Name
		isIndent = true
	}
	return &import_value{
		Name:     name,
		Path:     path,
		IsIndent: isIndent,
	}
}

type anotations []string

func (a anotations) append(s string) anotations {
	s = strings.TrimSpace(s)
	if s == "" {
		return a
	}
	return append(a, s)
}
func (a anotations) get_index(index int) string {
	if index >= len(a) {
		return ""
	}
	return a[index]
}
func (a anotations) fist() (string, bool) {
	if len(a) == 0 {
		return "", false
	}
	return a[0], true
}

type func_decl struct {
	Name               string                  //函数名
	NameFunc           string                  //函数名，可以被覆盖
	Receiver           string                  //接收者类型
	Comments_anotation map[string]anotations   //注解
	Comments           anotations              //注解
	Contents           anotations              //完全替代内容实现，不包含函数签名
	SetOptions         anotations              //设置选项
	In                 []*func_param_in_or_out //原始函数的入参
	Out                []*func_param_in_or_out //原始函数的返回值
	Return             []*func_param_in_or_out //生成函数的返回值
	Client             string
	Server             string
	Module             string
	IsAsync            bool
	ReqAppend          string
	ReqFirst           *func_param_in_or_out //第一个参数名
	RetFirst           *func_param_in_or_out //第一个返回值名
	CallOptionVar      string                //opt declaration variable name
	CallReqName        string                //请求参数变量名
	CallRetName        string                //返回值变量名
	CallOptionName     string
}

type func_param_in_or_out struct {
	Name            string
	ImportPre       string
	Type            string //*int ,int
	Type_is_pointer bool   //true, false 不是指针类型的，用作生成代码时声明
	Type_elem       string //int 不是指针类型的，用作生成代码时声明
	Type_elem_init  string //default "" {} ,非指针类型的怎么初始化 空串直接声明，{}是map，[]的初始化， 指针类型一律用这个类型初始化出来，然后取地址
}

var fix_import_pre_reg = regexp.MustCompile(`^[\[\]\.\*]*`)

func (this *func_param_in_or_out) fixFullTypeName() {
	if this.Type == "" {
		return
	}

	tmp_type := this.Type
	if strings.HasPrefix(tmp_type, "...") {
		tmp_type = strings.TrimPrefix(tmp_type, "...")
	}
	parts := strings.Split(tmp_type, ".")
	if len(parts) == 2 {
		this.ImportPre = parts[0]
		this.ImportPre = fix_import_pre_reg.ReplaceAllString(this.ImportPre, "")
	}

	this.Type_is_pointer = strings.HasPrefix(this.Type, "*")
	this.Type_elem = strings.TrimPrefix(this.Type, "*")
	if this.Type_is_pointer {
		name_temp := fmt.Sprintf("%s_temp", this.Name)
		if this.Type_elem_init != "" { //用注解制定了初始化的方式了
			init_code := fmt.Sprintf(this.Type_elem_init, name_temp)
			code := `%v
			%v = &%v`
			this.Type_elem_init = fmt.Sprintf(code, init_code, this.Name, name_temp)
			return
		}
		if strings.HasPrefix(this.Type_elem, "[]") || strings.HasPrefix(this.Type_elem, "map[") {
			code := `var %v = %v{}
			%v = &%v`
			this.Type_elem_init = fmt.Sprintf(code, name_temp, this.Type_elem, this.Name, name_temp)
		} else {
			code := `var %v %v
			%v = &%v`
			this.Type_elem_init = fmt.Sprintf(code, name_temp, this.Type_elem, this.Name, name_temp)
		}
	} else {
		if this.Type_elem_init != "" { //用注解制定了初始化的方式了
			this.Type_elem_init = fmt.Sprintf(this.Type_elem_init, this.Name)
			return
		}
		if strings.HasPrefix(this.Type_elem, "[]") || strings.HasPrefix(this.Type_elem, "map[") {
			this.Type_elem_init = fmt.Sprintf("%s = %v{}", this.Name, this.Type_elem)
		}
	}
}

func (this *func_param_in_or_out) String() string {
	return fmt.Sprintf("Name:%s,import_pre:%s,Type:%s", this.Name, this.ImportPre, this.Type)
}

var get_comment_key_value_reg = regexp.MustCompile(`^//\s*@crpc:\s*([^\s]+):\s*(.*)`)
var get_req_arg_type_reg = regexp.MustCompile(`^([^\s]+)[\s:]+([^\s]+)$`)

func get_req_arg_type(str string) (arg, type_str string, ok bool) {
	matches := get_req_arg_type_reg.FindStringSubmatch(str)
	if len(matches) == 3 {
		return matches[1], matches[2], true
	}
	return "", "", false
}

func get_comment_anotation_key_value(comment string) (string, string, bool) {
	matches := get_comment_key_value_reg.FindStringSubmatch(comment)
	if len(matches) == 3 {
		return matches[1], matches[2], true
	}
	return "", "", false
}

func handleFuncDecl(funcSpec *ast.FuncDecl, is_async bool) (res *func_decl) {

	if funcSpec.Type.Params == nil {
		return
	}

	var req_length int
	for _, param := range funcSpec.Type.Params.List {
		for range param.Names {
			req_length++
		}
	}
	if req_length > 2 || req_length < 1 {
		return
	}
	if funcSpec.Type.Results == nil {
		return
	}
	var res_length int
	for _, param := range funcSpec.Type.Results.List {
		if len(param.Names) == 0 {
			res_length++
			continue
		}
		for range param.Names {
			res_length++
		}
	}

	if res_length > 2 || res_length < 1 {
		return
	}

	res_len := len(funcSpec.Type.Results.List)
	res_last_type := getFieldType(funcSpec.Type.Results.List[res_len-1].Type)
	if res_last_type != "error" {
		return
	}

	name_func := funcSpec.Name.Name

	res = &func_decl{
		Name:               name_func,
		IsAsync:            is_async,
		Comments_anotation: map[string]anotations{},
	}
	// 打印函数的注释
	if funcSpec.Doc != nil {
		for _, comment := range funcSpec.Doc.List {
			if key, value, ok := get_comment_anotation_key_value(comment.Text); ok {
				res.Comments_anotation[key] = res.Comments_anotation[key].append(value)
			} else {
				res.Comments = res.Comments.append(comment.Text)
			}
		}
	}
	if v, ok := res.Comments_anotation[Annotation_IsSkip]; ok {
		if v1, ok1 := v.fist(); ok1 && !(v1 == "false" || v1 == "0" || v1 == "") { // 非0则为真
			return nil
		}
	}

	res.Contents, _ = res.Comments_anotation[Annotation_Content]
	res.SetOptions, _ = res.Comments_anotation[Annotation_SetOption]
	res.NameFunc, _ = res.Comments_anotation[Annotation_FuncName].append(name_func).fist()
	sub_sync, _ := res.Comments_anotation[Annotation_SubAsync].append(data.SubAsync).fist()
	if sub_sync != "" && is_async {
		res.NameFunc = res.NameFunc + sub_sync
	}
	res.Client, _ = res.Comments_anotation[Annotation_Client].append(data.Client).fist()
	res.Server, _ = res.Comments_anotation[Annotation_Server].append(data.Server).fist()
	res.Module, _ = res.Comments_anotation[Annotation_Module].append(data.Module).fist()
	res.ReqAppend, _ = res.Comments_anotation[Annotation_ReqAppend].append(data.Req_Append).fist()
	res.CallOptionName, _ = res.Comments_anotation[Annotation_CallOptionName].append(data.Call_Option_Name).fist()
	// 获取返回值信息
	for index, result := range funcSpec.Type.Results.List {
		res_type := getFieldType(result.Type)
		for _, name := range result.Names {
			v := &func_param_in_or_out{
				Name:           name.Name,
				Type:           res_type,
				Type_elem_init: "",
			}
			res.Out = append(res.Out, v)
		}
		// 如果没有命名返回值，打印类型
		if len(result.Names) == 0 {
			var name string
			if res_type == "error" {
				name = "err"
			} else {
				if index == 0 {
					name = "ret"
				} else {
					name = fmt.Sprintf("ret%d", index)
				}
			}
			v := &func_param_in_or_out{
				Name:           name,
				Type:           res_type,
				Type_elem_init: "",
			}
			res.Out = append(res.Out, v)
		}
	}
	type_elem_inits := res.Comments_anotation[Annotation_TypeElemInit]
	for index, out := range res.Out {
		out.Type_elem_init = type_elem_inits.get_index(index)
		out.fixFullTypeName()
	}

	if res_length > 1 {
		res.RetFirst = res.Out[0]
	}
	if res.RetFirst == nil {
		res.CallRetName = "nil"
	} else {
		if !res.RetFirst.Type_is_pointer {
			res.CallRetName = "&" + res.RetFirst.Name
		} else {
			res.CallRetName = res.RetFirst.Name
		}
	}
	if !res.IsAsync {
		res.Return = res.Out
	} else {
		v := &func_param_in_or_out{
			Name:           "call",
			Type:           "*crpc.Call",
			Type_elem_init: "",
		}
		v.fixFullTypeName()
		res.Return = []*func_param_in_or_out{v}
	}

	// 获取参数信息
	last_param_name := ""
	if funcSpec.Type.Params != nil {
		param_index := 0
		for _, param := range funcSpec.Type.Params.List {
			for _, name := range param.Names {
				//skip meta.Admin meta.Msg
				if req_length == 2 && param_index == 0 {
					param_index++
					continue
				}
				v := &func_param_in_or_out{
					Name: name.Name,
					Type: getFieldType(param.Type),
				}
				last_param_name = v.Name
				v.fixFullTypeName()
				res.In = append(res.In, v)
				param_index++
			}
		}
	}

	if req_append := res.ReqAppend; req_append != "" {
		req_append := strings.Trim(req_append, "\"")
		parts := strings.Split(req_append, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if arg, type_str, ok := get_req_arg_type(part); ok {
				v := &func_param_in_or_out{
					Name: arg,
					Type: type_str,
				}
				last_param_name = v.Name
				v.fixFullTypeName()
				res.In = append(res.In, v)
			}
		}
	}

	if res.CallOptionName == "" {
		res.CallOptionName = "opt"
	}
	if len(res.SetOptions) > 0 {
		var set_options []string
		for _, v := range res.SetOptions {
			set_options = append(set_options, v)
		}
		set_option_str := strings.Join(set_options, ".")
		if set_option_str != "" {
			set_option_str = "." + set_option_str
		}
		call_option_var := fmt.Sprintf("%v := crpc.Options()%v.Merge(%v...)", res.CallOptionName, set_option_str, last_param_name)
		res.CallOptionVar = call_option_var
	} else {
		res.CallOptionName = fmt.Sprintf("%v...", last_param_name)
	}

	if len(res.In) >= 1 {
		res.ReqFirst = res.In[0]
		res.CallReqName = res.ReqFirst.Name
	}
	if res.ReqFirst == nil {
		res.CallReqName = "nil"
	}
	return
}

// 获取字段类型的字符串表示
func getFieldType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident: // 基本类型或自定义类型
		return t.Name
	case *ast.SelectorExpr: // 包名.类型
		return fmt.Sprintf("%s.%s", t.X, t.Sel.Name)
	case *ast.ArrayType: // 数组类型
		return fmt.Sprintf("[]%s", getFieldType(t.Elt))
	case *ast.MapType: // map 类型
		return fmt.Sprintf("map[%s]%s", getFieldType(t.Key), getFieldType(t.Value))
	case *ast.StarExpr: // 指针类型
		return fmt.Sprintf("*%s", getFieldType(t.X))
	default:
		return fmt.Sprintf("%T", expr) // 其他类型
	}
}

//go:embed func_template.go.tmpl
var tmpl string
