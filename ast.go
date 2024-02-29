package main

import (
	"flag"
	"fmt"
	"github.com/archine/ast-base/core"
	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/jennifer/jen"
	"go/parser"
	"go/token"
	"io/fs"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Controller information
type controllerInfo struct {
	Name     string             // Controller struct name
	Pkg      string             // Package name
	IPath    string             // import path
	BasePath string             // Api base path
	apiCache []*core.MethodInfo // All apis of controller
}

var (
	// Controller root directory
	controllerRoot string
	// Application context path, all API paths are based on this path.
	// For example, when context is /user, the controller interface basePath is defined as /order, the specific API is /list, and the final result is /user/order/list
	contextPath string
	// Exclude some directories in the root path
	excludeDirs []string
	// Cache all controller information, key is controller name
	controllerCache map[string]*controllerInfo
	// common regex
	basePathRegex, restfulRegex, annoRegex, dirRegex *regexp.Regexp
)

func init() {
	readParameters()
	controllerCache = make(map[string]*controllerInfo)
	basePathRegex = regexp.MustCompile(`^//\s+@(BasePath)+[(]"(/.*)"[)]$`)
	restfulRegex = regexp.MustCompile(`^//\s+@(GET|POST|PUT|DELETE|HEAD|OPTIONS|PATCH)+[(]path="(/.*)"[)]`)
	annoRegex = regexp.MustCompile(`^//\s+(@[A-zA-z]+)\s*(->\s*(.*))*`)
	projectRoot, err := os.Getwd()
	if err != nil {
		log.Fatalf("init ast parser failure, %s", err.Error())
	}
	dirRegex = regexp.MustCompile(fmt.Sprintf("(.*)+(%s.*)/+(.*)", filepath.Base(projectRoot)))
}

// Read all command line parameters
func readParameters() {
	var excludeDir string
	flag.StringVar(&controllerRoot, "root", "controller", "API根目录.")
	flag.StringVar(&contextPath, "context", "/", "应用程序上下文路径")
	flag.StringVar(&excludeDir, "f", "", "需要过滤的目录,英文逗号分割, 如：dto,vo")
	flag.Parse()
	if len(excludeDir) > 0 {
		excludeDirs = strings.Split(excludeDir, ",")
	}
}

// Main Parse project controllers and API methods
// Run with go generate
func main() {
	controllerRootAbs, err := filepath.Abs(controllerRoot)
	if err != nil {
		log.Fatalf("[%s] failed to obtain the absolute path of the controller root directory, %s", controllerRootAbs, err.Error())
	}
	controllerRoot = path.Base(controllerRoot)
	set := token.NewFileSet()
	err = filepath.Walk(controllerRootAbs, func(path string, fileInfo fs.FileInfo, err error) error {
		// ignore directories and non-go files
		if fileInfo.IsDir() {
			if slices.Contains(excludeDirs, fileInfo.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(fileInfo.Name(), ".go") || fileInfo.Name() == "controller_init.go" {
			return nil
		}

		parseFile(strings.ReplaceAll(path, string(os.PathSeparator), "/"), set)
		return nil
	})
	if err != nil {
		log.Fatalf("parsing the project AST syntax tree failed, %s", err.Error())
	}
	recordControllerAndApi(controllerRootAbs)
}

// Parse AST of each go file
func parseFile(filePath string, fileSet *token.FileSet) {
	dFile, err := decorator.ParseFile(fileSet, filePath, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("faild to parse %s, %s", filePath, err.Error())
	}
	dst.Inspect(dFile, func(node dst.Node) bool {
		switch t := node.(type) {
		case *dst.GenDecl:
			var match bool
			var structType *dst.StructType
			var spec *dst.TypeSpec
			if spec, match = t.Specs[0].(*dst.TypeSpec); !match {
				return false
			}
			if structType, match = spec.Type.(*dst.StructType); !match {
				return false
			}
			if isController(structType.Fields.List) {
				ctrl := controllerCache[spec.Name.Name]
				if ctrl != nil {
					log.Fatalf("duplicate controller name: %s", spec.Name.Name)
				}
				ctrl = &controllerInfo{
					Pkg:  dFile.Name.Name,
					Name: spec.Name.Name,
				}
				controllerCache[ctrl.Name] = ctrl // cache current controller
				importPath := dirRegex.FindStringSubmatch(filePath)[2]
				if path.Base(importPath) != controllerRoot {
					ctrl.IPath = importPath
				}
				basePath := path.Join(contextPath)
				for _, comment := range t.Decs.Start {
					subMatch := basePathRegex.FindStringSubmatch(comment) // parse base path of current controller
					if len(subMatch) == 0 {
						continue
					}
					basePath = path.Join(basePath, subMatch[2])
					break
				}
				ctrl.BasePath = basePath
			}
		case *dst.FuncDecl:
			if t.Decs.Start == nil || t.Recv == nil || t.Name.Name == "PostConstruct" {
				return false
			}
			father := controllerCache[searchFather(t.Recv.List)] // which controller does it belong to
			if father == nil {
				log.Fatalf("[%s] API method father lost: %s", filePath, t.Name.Name)
			}
			var methods []*core.MethodInfo
			var annos map[string]string
			for _, comment := range t.Decs.Start {
				subMatch := restfulRegex.FindStringSubmatch(comment) // parse api path
				if len(subMatch) > 0 {
					if annos == nil {
						annos = make(map[string]string)
					}
					method := &core.MethodInfo{
						Name:        t.Name.Name,
						Annotations: annos,
					}
					if unicode.IsLower(rune(method.Name[0])) {
						log.Fatalf("[%s] invalid method name: [%s], must start with uppercase", filePath, method.Name)
					}
					method.Method = subMatch[1]
					method.ApiPath = path.Join(father.BasePath, subMatch[2])
					methods = append(methods, method)
					continue
				}
				subMatch = annoRegex.FindStringSubmatch(comment) // parse annotations
				if len(subMatch) > 0 && annos != nil {
					annos[subMatch[1]] = subMatch[3]
				}
			}
			if len(methods) > 0 {
				father.apiCache = append(father.apiCache, methods...)
			}
		}
		return true
	})
}

// Determines whether the current structure is a controller
func isController(fields []*dst.Field) bool {
	var ok bool
	var selectorExpr *dst.SelectorExpr
	for _, field := range fields {
		selectorExpr, ok = field.Type.(*dst.SelectorExpr)
		if !ok {
			continue
		}
		x := selectorExpr.X.(*dst.Ident)
		sel := selectorExpr.Sel
		if x.Name == "mvc" && sel.Name == "Controller" {
			ok = true
			break
		}
	}
	return ok
}

// Search the controller to which the method belongs
func searchFather(fields []*dst.Field) string {
	for _, field := range fields {
		if f, ok := field.Type.(*dst.StarExpr); ok {
			return f.X.(*dst.Ident).Name
		}
	}
	return ""
}

// All controller information and Api information for the current project is recorded here
func recordControllerAndApi(controllerAbs string) {
	if len(controllerCache) == 0 {
		return
	}
	newFile := jen.NewFile(controllerRoot)
	// generate remark
	newFile.HeaderComment("// 👉👉 Auto generate code by gp-ast framework, Do not edit!!! 🚫🚫")
	newFile.HeaderComment("// 👉👉 All controller information and Api information for the current project is recorded here.")
	newFile.HeaderComment(fmt.Sprintf("// ⏰⏰ %s\n", time.Now()))
	// generate import
	newFile.ImportName("github.com/archine/gin-plus/v3/mvc", "mvc")
	newFile.ImportName("github.com/archine/ast-base/core", "core")
	// controller that needs to be registered
	var registerCode []jen.Code
	// generate init func
	newFile.Func().Id("init").Params().Block(
		jen.Qual("github.com/archine/ast-base/core", "Apis").Op("=").
			Map(jen.String()).Index().Id("*").Qual("github.com/archine/ast-base/core", "MethodInfo").
			Values(jen.DictFunc(func(dict jen.Dict) {
				for _, ctrl := range controllerCache {
					// set the import and registration controller
					if ctrl.IPath == "" { // if the controller is in the root path, do not import the controller
						registerCode = append(registerCode, jen.Id(fmt.Sprintf("&%s{}", ctrl.Name)))
					} else {
						newFile.ImportName(ctrl.IPath, ctrl.Pkg)
						registerCode = append(registerCode, jen.Id("&").Qual(ctrl.IPath, ctrl.Name).Id("{}"))
					}

					dict[jen.Lit(ctrl.Name)] = jen.ValuesFunc(func(group *jen.Group) {
						for _, api := range ctrl.apiCache {
							group.Add(jen.Block(jen.Dict{
								jen.Id("Name"):    jen.Lit(api.Name),
								jen.Id("Method"):  jen.Lit(api.Method),
								jen.Id("ApiPath"): jen.Lit(api.ApiPath),
								jen.Id("Annotations"): jen.Map(jen.String()).String().Values(jen.DictFunc(func(dict jen.Dict) {
									for k, v := range api.Annotations {
										dict[jen.Lit(k)] = jen.Lit(v)
									}
								})),
							}))
						}
					})
				}
			})),
		// Start registering controllers to mvc
		jen.Qual("github.com/archine/gin-plus/v3/mvc", "Register").Call(registerCode...),
	)
	if err := newFile.Save(filepath.Join(controllerAbs, "controller_init.go")); err != nil {
		log.Fatalf("faild to generate controller_init.go, %s", err.Error())
	}
	log.Println("parse successfully")
}
