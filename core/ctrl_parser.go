package core

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/archine/gp-ast/v2/enum"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"unicode"

	"github.com/dave/dst"
)

// MethodInfo Api method info
type MethodInfo struct {
	Method  string // API method。such as: POST、GET、DELETE、PUT、OPTIONS、PATCH、HEAD
	APIPath string // API path
	Name    string // Func name
}

type CtrlInfo struct {
	BasePath string        `json:"-"`         // Api base path
	ApiCache []*MethodInfo `json:"api_cache"` // All apis of controller
}

type CtrlParser struct {
	contextPath   string
	ctrlCache     map[string]*CtrlInfo
	annoCache     map[string]map[string]string
	basePathRegex *regexp.Regexp
	restfulRegex  *regexp.Regexp
	annoRegex     *regexp.Regexp
}

func NewCtrlParser(contextPath string) *CtrlParser {
	p := &CtrlParser{
		contextPath:   contextPath,
		ctrlCache:     make(map[string]*CtrlInfo),
		annoCache:     make(map[string]map[string]string),
		basePathRegex: regexp.MustCompile(`^//\s+@(BasePath)+[(]"(/.*)"[)]$`),
		restfulRegex:  regexp.MustCompile(`^//\s+@(GET|POST|PUT|DELETE|HEAD|OPTIONS|PATCH)+[(]path="(/.*)"[)]`),
		annoRegex:     regexp.MustCompile(`^//\s+(@[A-zA-z]+)\s*(->\s*(.*))*`),
	}

	return p
}

func (p *CtrlParser) ParseStruct(genDecl *dst.GenDecl, structMeta *StructMeta) {
	ctrl := &CtrlInfo{}

	basePath := p.contextPath
	for _, comment := range genDecl.Decs.Start {
		if subMatch := p.basePathRegex.FindStringSubmatch(comment); len(subMatch) > 0 {
			basePath = path.Join(basePath, subMatch[2])
			break
		}
	}

	ctrl.BasePath = basePath
	p.ctrlCache[structMeta.Name] = ctrl
}

func (p *CtrlParser) ParseMethod(funcDecl *dst.FuncDecl) error {
	if funcDecl.Decs.Start == nil {
		return nil
	}

	// Find parent controller
	receiverName := searchReceiver(funcDecl.Recv.List)

	father := p.ctrlCache[receiverName]
	if father == nil {
		return nil
	}

	if len(funcDecl.Name.Name) > 0 && unicode.IsLower([]rune(funcDecl.Name.Name)[0]) {
		return fmt.Errorf("method %s must start with an uppercase letter", funcDecl.Name.Name)
	}

	var methods []*MethodInfo
	var annotations map[string]string

	// Parse method comments to extract API routes and annotation information
	for _, comment := range funcDecl.Decs.Start {
		if subMatch := p.restfulRegex.FindStringSubmatch(comment); len(subMatch) > 0 {
			method := &MethodInfo{
				Name:    funcDecl.Name.Name,
				Method:  subMatch[1],
				APIPath: path.Join(father.BasePath, subMatch[2]),
			}
			methods = append(methods, method)
		} else if subMatch = p.annoRegex.FindStringSubmatch(comment); len(subMatch) > 0 {
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations[subMatch[1]] = subMatch[3]
		}
	}

	// Cache methods and annotations
	if len(methods) > 0 {
		father.ApiCache = append(father.ApiCache, methods...)
		if len(annotations) > 0 {
			p.annoCache[methods[0].APIPath] = annotations
		}
	}

	return nil
}

func (p *CtrlParser) Generate(saveToPath string) error {
	if len(p.ctrlCache) == 0 {
		return nil
	}

	saveFullPath := filepath.Join(saveToPath, enum.ApiDefFile)
	file, err := os.OpenFile(saveFullPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	writeMap := map[string]any{
		"ctrl":       p.ctrlCache,
		"annotation": p.annoCache,
	}

	ctrlJsonBytes, _ := json.Marshal(writeMap)
	_, err = file.WriteString(base64.StdEncoding.EncodeToString(ctrlJsonBytes))
	return err
}

// searchReceiver extracts controller name from method receiver, supports pointer type receivers
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
