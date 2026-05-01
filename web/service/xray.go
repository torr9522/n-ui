package service

import (
	"encoding/json"
	"errors"
	"go.uber.org/atomic"
	"sync"
	"x-ui/database/model"
	"x-ui/logger"
	"x-ui/xray"
)

var p *xray.Process
var lock sync.Mutex
var isNeedXrayRestart atomic.Bool
var result string

type XrayService struct {
	inboundService InboundService
	settingService SettingService
}

type routingConfig struct {
	DomainStrategy string        `json:"domainStrategy,omitempty"`
	Rules          []routingRule `json:"rules,omitempty"`
}

type routingRule struct {
	Type        string   `json:"type,omitempty"`
	InboundTag  []string `json:"inboundTag,omitempty"`
	IP          []string `json:"ip,omitempty"`
	Protocol    []string `json:"protocol,omitempty"`
	OutboundTag string   `json:"outboundTag,omitempty"`
}

const dokodemoFreedomTag = "dokodemo-freedom"

func (s *XrayService) IsXrayRunning() bool {
	return p != nil && p.IsRunning()
}

func (s *XrayService) GetXrayErr() error {
	if p == nil {
		return nil
	}
	return p.GetErr()
}

func (s *XrayService) GetXrayResult() string {
	if result != "" {
		return result
	}
	if s.IsXrayRunning() {
		return ""
	}
	if p == nil {
		return ""
	}
	result = p.GetResult()
	return result
}

func (s *XrayService) GetXrayVersion() string {
	if p == nil {
		return "Unknown"
	}
	return p.GetVersion()
}

func (s *XrayService) GetXrayConfig() (*xray.Config, error) {
	templateConfig, err := s.settingService.GetXrayConfigTemplate()
	if err != nil {
		return nil, err
	}

	xrayConfig := &xray.Config{}
	err = json.Unmarshal([]byte(templateConfig), xrayConfig)
	if err != nil {
		return nil, err
	}
	accessIPService := AccessIPService{}
	err = accessIPService.ApplyAccessLogSetting(xrayConfig)
	if err != nil {
		return nil, err
	}

	inbounds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		return nil, err
	}
	allowPrivateOutboundNeeded := false
	for _, inbound := range inbounds {
		if !inbound.Enable {
			continue
		}
		inboundConfig := inbound.GenXrayInboundConfig()
		xrayConfig.InboundConfigs = append(xrayConfig.InboundConfigs, *inboundConfig)
		if inbound.Protocol == model.Dokodemo || inbound.Protocol == model.Tunnel {
			allowPrivateOutboundNeeded = true
		}
	}
	if allowPrivateOutboundNeeded {
		err = s.ensureDokodemoTunnelRouting(xrayConfig, inbounds)
		if err != nil {
			return nil, err
		}
	}
	return xrayConfig, nil
}

func (s *XrayService) ensureDokodemoTunnelRouting(xrayConfig *xray.Config, inbounds []*model.Inbound) error {
	routing := &routingConfig{}
	raw := string(xrayConfig.RouterConfig)
	if raw != "" && raw != "null" {
		if err := json.Unmarshal(xrayConfig.RouterConfig, routing); err != nil {
			return err
		}
	}

	tags := make([]string, 0)
	for _, inbound := range inbounds {
		if !inbound.Enable {
			continue
		}
		if inbound.Protocol == model.Dokodemo || inbound.Protocol == model.Tunnel {
			tags = append(tags, inbound.Tag)
		}
	}
	if len(tags) == 0 {
		return nil
	}

	if err := ensureTaggedFreedomOutbound(xrayConfig); err != nil {
		return err
	}

	rule := routingRule{
		Type:        "field",
		InboundTag:  tags,
		OutboundTag: dokodemoFreedomTag,
	}

	rules := make([]routingRule, 0, len(routing.Rules)+1)
	rules = append(rules, rule)
	rules = append(rules, routing.Rules...)
	routing.Rules = rules

	data, err := json.Marshal(routing)
	if err != nil {
		return err
	}
	xrayConfig.RouterConfig = data
	return nil
}

func ensureTaggedFreedomOutbound(xrayConfig *xray.Config) error {
	outbounds := make([]map[string]interface{}, 0)
	raw := string(xrayConfig.OutboundConfigs)
	if raw != "" && raw != "null" {
		if err := json.Unmarshal(xrayConfig.OutboundConfigs, &outbounds); err != nil {
			return err
		}
	}

	for _, outbound := range outbounds {
		if tag, ok := outbound["tag"].(string); ok && tag == dokodemoFreedomTag {
			return nil
		}
	}

	outbounds = append(outbounds, map[string]interface{}{
		"protocol": "freedom",
		"settings": map[string]interface{}{},
		"tag":      dokodemoFreedomTag,
	})

	data, err := json.Marshal(outbounds)
	if err != nil {
		return err
	}
	xrayConfig.OutboundConfigs = data
	return nil
}

func (s *XrayService) GetXrayTraffic() ([]*xray.Traffic, error) {
	if !s.IsXrayRunning() {
		return nil, errors.New("xray is not running")
	}
	return p.GetTraffic(true)
}

func (s *XrayService) RestartXray(isForce bool) error {
	lock.Lock()
	defer lock.Unlock()
	logger.Debug("restart xray, force:", isForce)

	xrayConfig, err := s.GetXrayConfig()
	if err != nil {
		return err
	}

	err = xray.TestConfig(xrayConfig)
	if err != nil {
		return err
	}

	var oldConfig *xray.Config
	if p != nil && p.IsRunning() {
		if !isForce && p.GetConfig().Equals(xrayConfig) {
			logger.Debug("not need to restart xray")
			return nil
		}
		oldConfig = p.GetConfig()
		p.Stop()
	}

	p = xray.NewProcess(xrayConfig)
	result = ""
	err = p.Start()
	if err == nil {
		return nil
	}
	if oldConfig != nil {
		logger.Warning("new xray start failed, trying rollback:", err)
		rollbackProcess := xray.NewProcess(oldConfig)
		rollbackErr := rollbackProcess.Start()
		if rollbackErr == nil {
			p = rollbackProcess
			return err
		}
		logger.Error("rollback xray start failed:", rollbackErr)
	}
	return err
}

func (s *XrayService) StopXray() error {
	lock.Lock()
	defer lock.Unlock()
	logger.Debug("stop xray")
	if s.IsXrayRunning() {
		return p.Stop()
	}
	return errors.New("xray is not running")
}

func (s *XrayService) SetToNeedRestart() {
	isNeedXrayRestart.Store(true)
}

func (s *XrayService) IsNeedRestartAndSetFalse() bool {
	return isNeedXrayRestart.CAS(true, false)
}
