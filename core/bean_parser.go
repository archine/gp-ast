package core

import (
	"fmt"
	"github.com/archine/gp-ast/v2/enum"
	"github.com/dave/jennifer/jen"
	"path/filepath"
	"time"
)

type BeanParser struct {
	beanCache map[string]*StructMeta
}

// NewBeanParser creates a new BeanParser instance
func NewBeanParser() *BeanParser {
	return &BeanParser{
		beanCache: make(map[string]*StructMeta),
	}
}

// ParseBean parses the given bean and stores its metadata
func (bp *BeanParser) ParseBean(structMeta *StructMeta) {
	if _, exists := bp.beanCache[structMeta.Name]; !exists {
		bp.beanCache[structMeta.Name] = structMeta
	}
}

// Generate BeanCode generates the code for all beans
func (bp *BeanParser) Generate(saveToPath string) error {
	if len(bp.beanCache) == 0 {
		return nil
	}

	newFile := jen.NewFile("main")
	addHeaderComments(newFile)
	addImports(newFile)

	// Build controller registration code
	var registerCode []jen.Code

	for _, bean := range bp.beanCache {
		if bean.IPath == "" {
			registerCode = append(registerCode, jen.Id(fmt.Sprintf("&%s{}", bean.Name)))
		} else {
			newFile.ImportName(bean.IPath, bean.Pkg)
			registerCode = append(registerCode, jen.Id("&").Qual(bean.IPath, bean.Name).Id("{}"))
		}
	}

	// Generate init function
	newFile.Func().Id("init").Params().Block(
		jen.Qual(enum.BeanImportPath, "SetBeans").Call(registerCode...),
	)

	saveFullPath := filepath.Join(saveToPath, enum.BeanInitFile)
	return newFile.Save(saveFullPath)
}

// addHeaderComments adds header comments to the generated file
func addHeaderComments(file *jen.File) {
	file.HeaderComment("// 👉👉 Auto generate code by gin-plus AST framework, Do not edit!!! 🚫🚫")
	file.HeaderComment("// 👉👉 All types are registered in the IoC container here.")
	file.HeaderComment(fmt.Sprintf("// ⏰⏰ %s\n", time.Now().Format("2006-01-02 15:04:05")))
}

// addImports adds necessary imports to the generated file
func addImports(file *jen.File) {
	file.ImportName(enum.BeanImportPath, "ioc")
}
