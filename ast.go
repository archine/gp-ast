package main

import (
	"flag"
	"fmt"
	"github.com/archine/gp-ast/v2/core"
	"github.com/archine/gp-ast/v2/enum"
	"github.com/archine/gp-ast/v2/util"
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
)

type astParser struct {
	scanPackages    []string
	scanSkips       []string
	appContext      string
	fileSet         *token.FileSet
	importPathRegex *regexp.Regexp
	ctrlParser      *core.CtrlParser
	beanParser      *core.BeanParser
}

func main() {
	var scanPkg, scanSkip, appContext string
	flag.StringVar(&scanPkg, "scan_pkg", ".", "Package names to scan, default '.' means scan all packages under current absolute path. Multiple package names separated by commas")
	flag.StringVar(&scanSkip, "scan_skip", "dto,vo", "Package names to skip during scanning, default 'dto,vo'. When the specified directory is at the top level, its subdirectories will also be ignored. Multiple package names separated by commas")
	flag.StringVar(&appContext, "context", "/", "Application context path, default '/'. If it doesn't start with '/', it will be automatically added.")
	flag.Parse()

	p := &astParser{
		fileSet: token.NewFileSet(),
	}

	if !strings.HasPrefix(appContext, "/") {
		appContext = "/" + appContext
	}
	p.appContext = path.Clean(appContext)

	if scanPkg != "" {
		for _, pkg := range util.SplitAndTrim(scanPkg) {
			pkgAbs, err := filepath.Abs(pkg)
			if err != nil {
				log.Fatalf("❌ Failed to resolve scan package path '%s': %s\nPlease check if the path exists or permissions are correct", pkg, err.Error())
			}
			if _, err = os.Stat(pkgAbs); os.IsNotExist(err) {
				log.Fatalf("❌ Specified scan package path does not exist: %s\nPlease confirm the path is correct", pkgAbs)
			}
			p.scanPackages = append(p.scanPackages, pkgAbs)
		}
	}

	if scanSkip != "" {
		p.scanSkips = append(p.scanSkips, util.SplitAndTrim(scanSkip)...)
	}

	pwd, _ := os.Getwd()
	p.ctrlParser = core.NewCtrlParser(p.appContext)
	p.beanParser = core.NewBeanParser()
	p.importPathRegex = regexp.MustCompile(fmt.Sprintf("(.*/)(%s.*)/(.*)", filepath.Base(pwd)))

	p.doScan()

	err := p.ctrlParser.Generate(pwd)
	if err != nil {
		log.Fatalf("❌ Failed to generate controller initialization code: %s\nPlease check generation directory permissions or confirm controller structure is correct", err.Error())
	}

	err = p.beanParser.Generate(pwd)
	if err != nil {
		log.Fatalf("❌ Failed to generate Bean initialization code: %s\nPlease check generation directory permissions or confirm Bean structure is correct", err.Error())
	}

	log.Printf("✅ Project parsing completed, initialization files saved to: %s", pwd)
}

func (p *astParser) doScan() {
	for _, scanPackage := range p.scanPackages {
		err := filepath.Walk(scanPackage, func(filePath string, fileInfo fs.FileInfo, err error) error {
			if err != nil {
				return fmt.Errorf("error accessing file %s: %w", filePath, err)
			}

			if strings.HasPrefix(filepath.Base(filePath), ".") {
				if fileInfo.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if fileInfo.IsDir() {
				if slices.Contains(p.scanSkips, fileInfo.Name()) {
					return filepath.SkipDir
				}
				return nil
			}

			if fileInfo.Name() == enum.BeanInitFile ||
				fileInfo.Name() == enum.ApiDefFile ||
				!strings.HasSuffix(fileInfo.Name(), ".go") ||
				strings.HasSuffix(filePath, "_test.go") {
				return nil
			}

			return p.parseFile(strings.ReplaceAll(filePath, string(os.PathSeparator), "/"))
		})

		if err != nil {
			log.Fatalf("❌ AST syntax analysis failed: %s", err.Error())
		}
	}
}

// parseFile processes each Go file
func (p *astParser) parseFile(filePath string) error {
	var dFile *dst.File
	dFile, err := decorator.ParseFile(p.fileSet, filePath, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse file %s: %s\nPlease check if file syntax is correct", filePath, err.Error())
	}

	var beanImportAlias, mvcImportAlias string
	var structFlag int // bit flags: 0=no relevant imports, 1=mvc, 2=bean

	// Scan import statements to identify MVC and Bean framework imports
	for _, importSpec := range dFile.Imports {
		importPath := strings.Trim(importSpec.Path.Value, "\"")
		if importPath == enum.MvcImportPath {
			structFlag |= enum.MvcFlag
			if importSpec.Name != nil {
				mvcImportAlias = importSpec.Name.Name
			} else {
				mvcImportAlias = "mvc"
			}
		}
		if importPath == enum.BeanImportPath {
			structFlag |= enum.BeanFlag
			if importSpec.Name != nil {
				beanImportAlias = importSpec.Name.Name
			} else {
				beanImportAlias = "ioc"
			}
			continue
		}
	}

	// If no relevant imports are found, skip further processing
	if structFlag == 0 {
		return nil
	}

	for _, decl := range dFile.Decls {
		switch node := decl.(type) {
		case *dst.GenDecl:
			if node.Tok == token.TYPE && len(node.Specs) > 0 {
				if structSpec, ok := node.Specs[0].(*dst.TypeSpec); ok {
					if structType, ok := structSpec.Type.(*dst.StructType); ok {
						// Safe import path extraction to prevent regex match failure
						var importPath string
						matches := p.importPathRegex.FindStringSubmatch(filePath)
						if len(matches) > 2 {
							importPath = matches[2]
						}

						structMeta := &core.StructMeta{
							Name:  structSpec.Name.Name,
							Pkg:   dFile.Name.Name,
							IPath: importPath,
						}

						for _, field := range structType.Fields.List {
							selectorExpr, ok := field.Type.(*dst.SelectorExpr)
							if !ok {
								continue
							}

							x, ok := selectorExpr.X.(*dst.Ident)
							if !ok {
								continue
							}

							switch {
							case x.Name == mvcImportAlias && selectorExpr.Sel.Name == "Controller":
								p.ctrlParser.ParseStruct(node, structMeta)
								p.beanParser.ParseBean(structMeta)
							case x.Name == beanImportAlias && selectorExpr.Sel.Name == "Bean":
								p.beanParser.ParseBean(structMeta)
							}
						}
					}
				}
			}
		case *dst.FuncDecl:
			if node.Recv == nil || len(node.Recv.List) == 0 {
				continue
			}

			if structFlag&enum.MvcFlag != 0 {
				if err = p.ctrlParser.ParseMethod(node); err != nil {
					return fmt.Errorf("failed to parse controller method in file %s: %w", filePath, err)
				}
			}
		}
	}

	return nil
}
