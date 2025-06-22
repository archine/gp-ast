package enum

const (
	BeanFlag = 1 << iota // Bean struct flag
	MvcFlag              // Mvc struct flag

	BeanInitFile = "gp_bean_init.go" // Bean initialization file name
	ApiDefFile   = "gp_api.def"      // API definition file name

	BeanImportPath = "github.com/archine/gin-plus/v4/ioc" // Bean import path
	MvcImportPath  = "github.com/archine/gin-plus/v4/mvc" // Mvc import path
)
