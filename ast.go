package main

import (
	"flag"
	"fmt"
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

	"github.com/archine/ast-base"
	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/jennifer/jen"
)

const (
	generatedFileName   = "controller_init.go"
	mvcPackage          = "mvc"
	controllerField     = "Controller"
	postConstructMethod = "PostConstruct"

	// Regular expression patterns
	basePathPattern = `^//\s+@(BasePath)+[(]"(/.*)"[)]$`
	restfulPattern  = `^//\s+@(GET|POST|PUT|DELETE|HEAD|OPTIONS|PATCH)+[(]path="(/.*)"[)]`
	annoPattern     = `^//\s+(@[A-zA-z]+)\s*(->\s*(.*))*`
)

// Controller information
type controllerInfo struct {
	Name     string                 // Controller struct name
	Pkg      string                 // Package name
	IPath    string                 // import path
	BasePath string                 // Api base path
	apiCache []*ast_base.MethodInfo // All apis of controller
}

// parser is responsible for parsing the project structure and generating code
type astParser struct {
	controllerRoot  string
	contextPath     string
	excludeDirs     []string
	controllerCache map[string]*controllerInfo
	annoCache       map[string]map[string]string
	basePathRegex   *regexp.Regexp
	restfulRegex    *regexp.Regexp
	annoRegex       *regexp.Regexp
	dirRegex        *regexp.Regexp
}

// creates a new parser instance
func newParser() *astParser {
	p := &astParser{
		controllerCache: make(map[string]*controllerInfo),
		annoCache:       make(map[string]map[string]string),
	}
	p.readParameters()
	p.initRegexes()
	return p
}

// Initialize parameters and regex patterns
func (p *astParser) readParameters() {
	var excludeDir string
	flag.StringVar(&p.controllerRoot, "root", "controller", "API根目录.")
	flag.StringVar(&p.contextPath, "context", "/", "应用程序上下文路径, / 开头")
	flag.StringVar(&excludeDir, "f", "", "需要过滤的目录,英文逗号分割,且不要存在空格, 如：dto,vo")
	flag.Parse()

	if !strings.HasPrefix(p.contextPath, "/") {
		p.contextPath = "/" + p.contextPath
	}

	if len(excludeDir) > 0 {
		p.excludeDirs = strings.Split(excludeDir, ",")
	}
}

func (p *astParser) initRegexes() {
	p.basePathRegex = regexp.MustCompile(basePathPattern)
	p.restfulRegex = regexp.MustCompile(restfulPattern)
	p.annoRegex = regexp.MustCompile(annoPattern)

	projectRoot, err := os.Getwd()
	if err != nil {
		log.Fatalf("init ast parser failure, %s", err.Error())
	}
	p.dirRegex = regexp.MustCompile(fmt.Sprintf("(.*)+(%s.*)/+(.*)", filepath.Base(projectRoot)))
}

// Parse project controllers and API methods
func main() {
	newParser().parse()
}

// Parse executes the main parsing logic
func (p *astParser) parse() {
	controllerRootAbs, err := filepath.Abs(p.controllerRoot)
	if err != nil {
		log.Fatalf("failed to get absolute path: %s", err.Error())
	}

	if _, err = os.Stat(controllerRootAbs); os.IsNotExist(err) {
		log.Fatalf("controller directory does not exist: %s", controllerRootAbs)
	}

	p.controllerRoot = path.Base(p.controllerRoot)
	fileSet := token.NewFileSet()

	err = filepath.Walk(controllerRootAbs, func(filePath string, fileInfo fs.FileInfo, err error) error {
		// Skip directories and handle exclusions
		if fileInfo.IsDir() {
			if slices.Contains(p.excludeDirs, fileInfo.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip non-go files and generated files
		if !strings.HasSuffix(fileInfo.Name(), ".go") || fileInfo.Name() == generatedFileName {
			return nil
		}

		p.parseFile(strings.ReplaceAll(filePath, string(os.PathSeparator), "/"), fileSet)
		return nil
	})

	if err != nil {
		log.Fatalf("parsing the project AST syntax tree failed, %s", err.Error())
	}

	p.generateCode(controllerRootAbs)
}

// Parse AST of each go file
func (p *astParser) parseFile(filePath string, fileSet *token.FileSet) {
	dFile, err := decorator.ParseFile(fileSet, filePath, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("failed to parse %s, %s", filePath, err.Error())
	}

	dst.Inspect(dFile, func(node dst.Node) bool {
		switch t := node.(type) {
		case *dst.GenDecl:
			p.handleControllerStruct(t, dFile, filePath)
		case *dst.FuncDecl:
			p.handleAPIMethod(t, filePath)
		}
		return true
	})
}

// Handle controller struct declarations
func (p *astParser) handleControllerStruct(genDecl *dst.GenDecl, dFile *dst.File, filePath string) {
	if len(genDecl.Specs) == 0 {
		return
	}

	spec, ok := genDecl.Specs[0].(*dst.TypeSpec)
	if !ok {
		return
	}

	structType, ok := spec.Type.(*dst.StructType)
	if !ok || !isController(structType.Fields.List) {
		return
	}

	// Check for duplicate controller names
	if _, exists := p.controllerCache[spec.Name.Name]; exists {
		log.Fatalf("[%s] duplicate controller name: %s", filePath, spec.Name.Name)
	}

	// Create controller info
	ctrl := &controllerInfo{
		Pkg:  dFile.Name.Name,
		Name: spec.Name.Name,
	}

	// Set import path
	importPath := p.dirRegex.FindStringSubmatch(filePath)[2]
	if path.Base(importPath) != p.controllerRoot {
		ctrl.IPath = importPath
	}

	// Set base path from comments
	basePath := p.contextPath
	for _, comment := range genDecl.Decs.Start {
		if subMatch := p.basePathRegex.FindStringSubmatch(comment); len(subMatch) > 0 {
			basePath = path.Join(basePath, subMatch[2])
			break
		}
	}
	ctrl.BasePath = basePath

	p.controllerCache[ctrl.Name] = ctrl
}

// Handle API method declarations
func (p *astParser) handleAPIMethod(funcDecl *dst.FuncDecl, filePath string) {
	if funcDecl.Decs.Start == nil || funcDecl.Recv == nil || funcDecl.Name.Name == postConstructMethod {
		return
	}

	// Find parent controller
	receiverName := searchReceiver(funcDecl.Recv.List)
	father := p.controllerCache[receiverName]
	if father == nil {
		log.Printf("Warning: [%s] method [%s] receiver [%s] not found in controller cache, skipping\n", filePath, funcDecl.Name.Name, receiverName)
		return
	}

	// Parse method comments
	var methods []*ast_base.MethodInfo
	var annotations map[string]string

	for _, comment := range funcDecl.Decs.Start {
		// Try to parse as API method
		if subMatch := p.restfulRegex.FindStringSubmatch(comment); len(subMatch) > 0 {
			if unicode.IsLower(rune(funcDecl.Name.Name[0])) {
				log.Fatalf("[%s] invalid method name: [%s], must start with uppercase", filePath, funcDecl.Name.Name)
			}

			method := &ast_base.MethodInfo{
				Name:    funcDecl.Name.Name,
				Method:  subMatch[1],
				APIPath: path.Join(father.BasePath, subMatch[2]),
			}
			methods = append(methods, method)
		} else if subMatch = p.annoRegex.FindStringSubmatch(comment); len(subMatch) > 0 {
			// Parse annotations
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations[subMatch[1]] = subMatch[3]
		}
	}

	// Cache methods and annotations
	if len(methods) > 0 {
		father.apiCache = append(father.apiCache, methods...)
		if len(annotations) > 0 {
			p.annoCache[methods[0].APIPath] = annotations
		}
	}
}

// Generate the controller initialization code
func (p *astParser) generateCode(controllerAbs string) {
	if len(p.controllerCache) == 0 {
		return
	}

	newFile := jen.NewFile(p.controllerRoot)
	addHeaderComments(newFile)
	addImports(newFile)

	// Build controller registration code
	var registerCode []jen.Code
	for _, ctrl := range p.controllerCache {
		if ctrl.IPath == "" {
			registerCode = append(registerCode, jen.Id(fmt.Sprintf("&%s{}", ctrl.Name)))
		} else {
			newFile.ImportName(ctrl.IPath, ctrl.Pkg)
			registerCode = append(registerCode, jen.Id("&").Qual(ctrl.IPath, ctrl.Name).Id("{}"))
		}
	}

	// Generate init function
	newFile.Func().Id("init").Params().Block(
		// Set metadata APIs
		jen.Qual("github.com/archine/ast-base", "Result.Apis").Op("=").
			Map(jen.String()).Index().Id("*").Qual("github.com/archine/ast-base", "MethodInfo").
			Values(jen.DictFunc(func(dict jen.Dict) {
				for _, ctrl := range p.controllerCache {
					dict[jen.Lit(ctrl.Name)] = jen.ValuesFunc(func(group *jen.Group) {
						for _, api := range ctrl.apiCache {
							group.Add(jen.Block(jen.Dict{
								jen.Id("Name"):    jen.Lit(api.Name),
								jen.Id("Method"):  jen.Lit(api.Method),
								jen.Id("APIPath"): jen.Lit(api.APIPath),
							}))
						}
					})
				}
			})),
		// Set annotations
		jen.Qual("github.com/archine/gin-plus/v3/mvc", "SetAnnotations").Call(
			jen.Map(jen.String()).Map(jen.String()).String().
				Values(jen.DictFunc(func(dict jen.Dict) {
					for k, v := range p.annoCache {
						dict[jen.Lit(k)] = jen.ValuesFunc(func(group *jen.Group) {
							for anno, value := range v {
								group.Add(jen.Dict{
									jen.Lit(anno): jen.Lit(value),
								})
							}
						})
					}
				}))),
		// Register controllers
		jen.Qual("github.com/archine/gin-plus/v3/mvc", "Register").Call(registerCode...),
	)

	if err := newFile.Save(filepath.Join(controllerAbs, generatedFileName)); err != nil {
		log.Fatalf("failed to generate %s, %s", generatedFileName, err.Error())
	}

	log.Println("Controller initialization code generated successfully:", filepath.Join(controllerAbs, generatedFileName))
}

// Find the controller name from method receiver
func searchReceiver(fields []*dst.Field) string {
	for _, field := range fields {
		if starExpr, ok := field.Type.(*dst.StarExpr); ok {
			if ident, ok := starExpr.X.(*dst.Ident); ok {
				return ident.Name
			}
		}
	}
	return ""
}

// Check if struct is a controller
func isController(fields []*dst.Field) bool {
	for _, field := range fields {
		if selectorExpr, ok := field.Type.(*dst.SelectorExpr); ok {
			if x, ok := selectorExpr.X.(*dst.Ident); ok {
				if x.Name == mvcPackage && selectorExpr.Sel.Name == controllerField {
					return true
				}
			}
		}
	}
	return false
}

// addHeaderComments adds header comments to the generated file
func addHeaderComments(file *jen.File) {
	file.HeaderComment("// 👉👉 Auto generate code by gp-ast framework, Do not edit!!! 🚫🚫")
	file.HeaderComment("// 👉👉 All controller information and Api information for the current project is recorded here.")
	file.HeaderComment(fmt.Sprintf("// ⏰⏰ %s\n", time.Now()))
}

// addImports adds necessary imports to the generated file
func addImports(file *jen.File) {
	file.ImportName("github.com/archine/gin-plus/v3/mvc", "mvc")
	file.ImportName("github.com/archine/ioc", "ioc")
	file.ImportName("github.com/gin-gonic/gin", "gin")
	file.ImportName("github.com/archine/ast-base", "ast_base")
}
