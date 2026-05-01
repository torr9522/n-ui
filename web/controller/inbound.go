package controller

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"strconv"
	"x-ui/database/model"
	"x-ui/logger"
	"x-ui/web/global"
	"x-ui/web/service"
	"x-ui/web/session"
)

type InboundController struct {
	inboundService service.InboundService
	xrayService    service.XrayService
	portLimit      service.PortLimitService
}

func NewInboundController(g *gin.RouterGroup) *InboundController {
	a := &InboundController{}
	a.initRouter(g)
	a.startTask()
	return a
}

func (a *InboundController) initRouter(g *gin.RouterGroup) {
	g = g.Group("/inbound")

	g.POST("/list", a.getInbounds)
	g.POST("/add", a.addInbound)
	g.POST("/del/:id", a.delInbound)
	g.POST("/update/:id", a.updateInbound)
	g.POST("/portlimit/sync", a.syncPortLimit)
	g.POST("/portlimit/logs", a.getPortLimitLogs)
	g.POST("/portlimit/status", a.getPortLimitStatus)
	g.POST("/portlimit/diag", a.getPortLimitDiag)
}

func (a *InboundController) startTask() {
	webServer := global.GetWebServer()
	c := webServer.GetCron()
	c.AddFunc("@every 10s", func() {
		if a.xrayService.IsNeedRestartAndSetFalse() {
			err := a.xrayService.RestartXray(false)
			if err != nil {
				logger.Error("restart xray failed:", err)
			}
		}
	})
}

func (a *InboundController) getInbounds(c *gin.Context) {
	user := session.GetLoginUser(c)
	inbounds, err := a.inboundService.GetInbounds(user.Id)
	if err != nil {
		jsonMsg(c, "get", err)
		return
	}
	jsonObj(c, inbounds, nil)
}

func (a *InboundController) addInbound(c *gin.Context) {
	inbound := &model.Inbound{}
	err := c.ShouldBind(inbound)
	if err != nil {
		jsonMsg(c, "add", err)
		return
	}
	user := session.GetLoginUser(c)
	inbound.UserId = user.Id
	inbound.Enable = true
	inbound.Tag = fmt.Sprintf("inbound-%v", inbound.Port)
	err = a.inboundService.AddInbound(inbound)
	jsonMsg(c, "add", err)
	if err == nil {
		a.xrayService.SetToNeedRestart()
		a.portLimit.SyncNow()
	}
}

func (a *InboundController) delInbound(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "delete", err)
		return
	}
	err = a.inboundService.DelInbound(id)
	jsonMsg(c, "delete", err)
	if err == nil {
		a.xrayService.SetToNeedRestart()
		a.portLimit.SyncNow()
	}
}

func (a *InboundController) updateInbound(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		jsonMsg(c, "update", err)
		return
	}
	inbound := &model.Inbound{
		Id: id,
	}
	err = c.ShouldBind(inbound)
	if err != nil {
		jsonMsg(c, "update", err)
		return
	}
	err = a.inboundService.UpdateInbound(inbound)
	jsonMsg(c, "update", err)
	if err == nil {
		a.xrayService.SetToNeedRestart()
		a.portLimit.SyncNow()
	}
}

func (a *InboundController) syncPortLimit(c *gin.Context) {
	output, err := a.portLimit.SyncNowBlocking()
	if err != nil {
		jsonMsg(c, "sync port-limit", err)
		return
	}
	jsonObj(c, output, nil)
}

func (a *InboundController) getPortLimitLogs(c *gin.Context) {
	logs, err := a.portLimit.GetRecentLogs(120)
	if err != nil {
		jsonMsg(c, "get port-limit logs", err)
		return
	}
	jsonObj(c, logs, nil)
}

func (a *InboundController) getPortLimitStatus(c *gin.Context) {
	rules, err := a.portLimit.GetRuleStatus()
	if err != nil {
		jsonMsg(c, "get port-limit status", err)
		return
	}
	jsonObj(c, rules, nil)
}

func (a *InboundController) getPortLimitDiag(c *gin.Context) {
	diag, err := a.portLimit.BuildDiagnostics()
	if err != nil {
		jsonMsg(c, "build port-limit diagnostics", err)
		return
	}
	jsonObj(c, diag, nil)
}
