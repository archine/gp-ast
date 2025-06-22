package gp_ast

import (
	"flag"
	"fmt"
	"github.com/archine/gp-ast/v2/core"
	"github.com/archine/gp-ast/v2/enum"
	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
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
	"unicode"
)

type AstParser struct {
	scanPackages    []string
	scanSkips       []string
	appContext      string
	fileSet         *token.FileSet
	importPathRegex *regexp.Regexp
	ctrlParser      *core.CtrlParser
	beanParser      *core.BeanParser
}

func Main() {
	var scanPkg, scanSkip, appContext string
	flag.StringVar(&scanPkg, "scan_pkg", ".", "扫描的包名, 默认'.' 表示扫描当前绝对路径下的所有包。多个包名用英文逗号分隔")
	flag.StringVar(&scanSkip, "scan_skip", "", "扫描时要跳过的包名，默认 'dto,vo,po'。当指定的目录为最顶层，那么其子目录也会被忽略。多个包名用英文逗号分隔")
	flag.StringVar(&appContext, "context", "/", "应用的上下文路径, 默认 '/'，如果不以 '/' 开头会自动补充。")
	flag.Parse()

	astParser := &AstParser{
		fileSet: token.NewFileSet(),
	}

	if !strings.HasPrefix(appContext, "/") {
		appContext = "/" + appContext
	}
	astParser.appContext = path.Clean(appContext)

	if scanPkg != "" {
		split := strings.Split(scanPkg, ",")
		for _, pkg := range split {
			pkg = strings.TrimSpace(pkg)
			pkg = strings.Trim(pkg, "/")
			if pkg != "" {
				pkgAbs, err := filepath.Abs(pkg)
				if err != nil {
					log.Fatalf("failed to get absolute path for scan package: %s, error: %s", pkg, err.Error())
				}
				if _, err = os.Stat(pkgAbs); os.IsNotExist(err) {
					log.Fatalf("scan package does not exist: %s", pkgAbs)
				}
				astParser.scanPackages = append(astParser.scanPackages, pkgAbs)
			}
		}
	}

	if scanSkip != "" {
		split := strings.Split(scanSkip, ",")
		for _, skip := range split {
			skip = strings.TrimSpace(skip)
			skip = strings.Trim(skip, "/")
			if skip != "" {
				astParser.scanSkips = append(astParser.scanSkips, skip)
			}
		}
	}

	pwd, _ := os.Getwd()
	astParser.ctrlParser = core.NewCtrlParser(astParser.appContext)
	astParser.beanParser = core.NewBeanParser()
	astParser.importPathRegex = regexp.MustCompile(fmt.Sprintf("(.*/)(%s.*)/(.*)", filepath.Base(pwd)))

	astParser.doScan()

	err := astParser.ctrlParser.Generate(pwd)
	if err != nil {
		log.Fatalf("生成控制器初始化代码失败: %s", err.Error())
	}

	err = astParser.beanParser.Generate(pwd)
	if err != nil {
		log.Fatalf("生成 Bean 初始化代码失败: %s", err.Error())
	}

	log.Printf("项目解析完成，初始化文件已保存至: %s", pwd)
}

func (p *AstParser) doScan() {
	for _, scanPackage := range p.scanPackages {
		err := filepath.Walk(scanPackage, func(filePath string, fileInfo fs.FileInfo, err error) error {
			// Skip if there's an error accessing the file
			if strings.HasPrefix(filePath, ".") {
				return filepath.SkipDir
			}

			if fileInfo.IsDir() {
				// Skip directories that match the skip list
				if slices.Contains(p.scanSkips, fileInfo.Name()) {
					return filepath.SkipDir // Skip this directory and its subdirectories
				}
				return nil
			}

			// Skip non-Go files, generated files, and test files
			if fileInfo.Name() == enum.BeanInitFile ||
				fileInfo.Name() == enum.ApiDefFile ||
				!strings.HasSuffix(fileInfo.Name(), ".go") ||
				strings.HasSuffix(filePath, "_test.go") {

				return nil
			}

			return p.parseFile(strings.ReplaceAll(filePath, string(os.PathSeparator), "/"))
		})

		if err != nil {
			log.Fatalf("analyze the AST syntax error, %s", err.Error())
		}
	}
}

// parseFile processes each Go file
func (p *AstParser) parseFile(filePath string) error {
	var dFile *dst.File
	dFile, err := decorator.ParseFile(p.fileSet, filePath, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse %s, %s", filePath, err.Error())
	}

	var beanImportAlias, mvcImportAlias string
	var structFlag int // 0: no struct, 1: mvc, 2: bean

	for _, importSpec := range dFile.Imports {
		importPath := strings.Trim(importSpec.Path.Value, "\"")
		if importPath == enum.BeanImportPath {
			structFlag |= enum.BeanFlag
			if importSpec.Name != nil {
				beanImportAlias = importSpec.Name.Name
			} else {
				beanImportAlias = "ioc"
			}
			continue
		}
		if importPath == enum.MvcImportPath {
			structFlag |= enum.MvcFlag
			if importSpec.Name != nil {
				mvcImportAlias = importSpec.Name.Name
			} else {
				mvcImportAlias = "mvc"
			}
		}
	}
	// 如果当前文件没有导入 mvc 或 bean 包，则不需要解析
	if structFlag == 0 {
		return nil
	}

	for _, decl := range dFile.Decls {
		switch node := decl.(type) {
		case *dst.GenDecl:
			if node.Tok == token.TYPE {
				if len(node.Specs) > 0 {
					if structSpec, ok := node.Specs[0].(*dst.TypeSpec); ok {
						if structType, ok := structSpec.Type.(*dst.StructType); ok {
							structMeta := &core.StructMeta{
								Name:  structSpec.Name.Name,
								Pkg:   dFile.Name.Name,
								IPath: p.importPathRegex.FindStringSubmatch(filePath)[2],
							}

							for _, field := range structType.Fields.List {
								if selectorExpr, ok := field.Type.(*dst.SelectorExpr); ok {
									if x, ok := selectorExpr.X.(*dst.Ident); ok {
										if x.Name == mvcImportAlias && selectorExpr.Sel.Name == "Controller" {
											p.ctrlParser.ParseStruct(node, structMeta)
											p.beanParser.ParseBean(structMeta)
											continue
										}
										if x.Name == beanImportAlias && selectorExpr.Sel.Name == "Bean" {
											p.beanParser.ParseBean(structMeta)
										}
									}
								}
							}
						}
					}
				}
			}
		case *dst.FuncDecl:
			if node.Recv != nil && len(node.Recv.List) > 0 || unicode.IsLower([]rune(node.Name.Name)[0]) {
				if structFlag&enum.MvcFlag != 0 {
					p.ctrlParser.ParseMethod(node)
				}
			}
		default:

		}
	}

	return nil
}
