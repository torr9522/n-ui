package controller

import (
	"github.com/gin-gonic/gin"
	"x-ui/web/service"
)

type AccessIPController struct {
	accessIPService service.AccessIPService
	xrayService     service.XrayService
}

func NewAccessIPController(g *gin.RouterGroup) *AccessIPController {
	a := &AccessIPController{}
	a.initRouter(g)
	return a
}

func (a *AccessIPController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/access-ip")
	g.POST("/config", a.getConfig)
	g.POST("/config/update", a.updateConfig)
	g.POST("/list", a.list)
	g.POST("/clear", a.clear)
}

func (a *AccessIPController) getConfig(c *gin.Context) {
	config, err := a.accessIPService.GetConfig()
	if err != nil {
		jsonMsg(c, "get access ip config", err)
		return
	}
	jsonObj(c, config, nil)
}

func (a *AccessIPController) updateConfig(c *gin.Context) {
	config := &service.AccessIPConfig{}
	err := c.ShouldBind(config)
	if err != nil {
		jsonMsg(c, "update access ip config", err)
		return
	}
	needRestart, err := a.accessIPService.UpdateConfig(config)
	if err == nil && needRestart {
		err = a.xrayService.RestartXray(true)
	}
	jsonMsg(c, "update access ip config", err)
}

func (a *AccessIPController) list(c *gin.Context) {
	records, err := a.accessIPService.GetAccessIPRecords()
	if err != nil {
		jsonMsg(c, "get access ip records", err)
		return
	}
	jsonObj(c, records, nil)
}

func (a *AccessIPController) clear(c *gin.Context) {
	err := a.accessIPService.ClearAccessIPRecords()
	jsonMsg(c, "clear access ip records", err)
}
