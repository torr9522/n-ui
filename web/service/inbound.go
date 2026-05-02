package service

import (
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"regexp"
	"strings"
	"time"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/util/common"
	"x-ui/xray"
)

type InboundService struct {
}

type trojanInboundSettings struct {
	Clients   []map[string]interface{} `json:"clients"`
	Fallbacks []map[string]interface{} `json:"fallbacks"`
}

type vlessInboundSettings struct {
	Vlesses    []map[string]interface{} `json:"clients"`
	Decryption string                   `json:"decryption"`
	Fallbacks  []map[string]interface{} `json:"fallbacks"`
}

type socksInboundSettings struct {
	Auth     string                   `json:"auth"`
	Accounts []map[string]interface{} `json:"accounts"`
	Udp      interface{}              `json:"udp"`
	IP       string                   `json:"ip"`
}

var rateLimitRegexp = regexp.MustCompile(`(?i)^(\d+(?:\.\d+)?)(kbit|mbit|gbit|kbps|mbps|gbps|kbyte(?:/s)?|mbyte(?:/s)?|gbyte(?:/s)?)?$`)

func normalizeRate(rate string) string {
	rate = strings.TrimSpace(strings.ToLower(rate))
	rate = strings.ReplaceAll(rate, " ", "")
	if rate == "" {
		return ""
	}

	matches := rateLimitRegexp.FindStringSubmatch(rate)
	if len(matches) == 0 {
		return rate
	}

	unit := matches[2]
	if unit == "" {
		unit = "mbit"
	}
	switch unit {
	case "kbyte", "mbyte", "gbyte":
		unit += "/s"
	}

	return matches[1] + unit
}

func (s *InboundService) normalizeLimit(inbound *model.Inbound) {
	if inbound.IPLimit < 0 {
		inbound.IPLimit = 0
	}
	if inbound.IPTimeout <= 0 {
		inbound.IPTimeout = 5
	}
	if inbound.IPTimeout > 1440 {
		inbound.IPTimeout = 1440
	}
	inbound.PortRate = normalizeRate(inbound.PortRate)
	inbound.IPRate = normalizeRate(inbound.IPRate)
	if inbound.PortRate != "" && !rateLimitRegexp.MatchString(inbound.PortRate) {
		inbound.PortRate = ""
	}
	if inbound.IPRate != "" && !rateLimitRegexp.MatchString(inbound.IPRate) {
		inbound.IPRate = ""
	}
}

func (s *InboundService) normalizeProtocolSettings(inbound *model.Inbound) {
	settings := strings.TrimSpace(inbound.Settings)
	if settings == "" {
		return
	}

	switch inbound.Protocol {
	case model.Trojan:
		var trojan trojanInboundSettings
		if err := json.Unmarshal([]byte(settings), &trojan); err != nil {
			return
		}
		for _, client := range trojan.Clients {
			delete(client, "flow")
		}
		data, err := json.Marshal(trojan)
		if err != nil {
			return
		}
		inbound.Settings = string(data)
	case model.VLESS:
		var vless vlessInboundSettings
		if err := json.Unmarshal([]byte(settings), &vless); err != nil {
			return
		}
		for _, client := range vless.Vlesses {
			if flow, ok := client["flow"].(string); ok {
				if flow == "xtls-rprx-direct" || flow == "xtls-rprx-origin" {
					client["flow"] = ""
				}
			}
		}
		data, err := json.Marshal(vless)
		if err != nil {
			return
		}
		inbound.Settings = string(data)
	case model.Socks, model.Mixed:
		var socks socksInboundSettings
		if err := json.Unmarshal([]byte(settings), &socks); err != nil {
			return
		}
		socks.Auth = normalizeSocksAuth(socks.Auth, len(socks.Accounts) > 0)
		socks.Udp = normalizeSocksUDPValue(socks.Udp)
		if strings.TrimSpace(socks.IP) == "" {
			socks.IP = "127.0.0.1"
		}
		if socks.Auth != "password" {
			socks.Accounts = nil
		}
		data, err := json.Marshal(socks)
		if err != nil {
			return
		}
		inbound.Settings = string(data)
	default:
		return
	}
}

func normalizeSocksAuth(auth string, hasAccounts bool) string {
	auth = strings.TrimSpace(strings.ToLower(auth))
	switch auth {
	case "password":
		return "password"
	case "noauth":
		return "noauth"
	default:
		if hasAccounts {
			return "password"
		}
		return "noauth"
	}
}

func normalizeSocksUDPValue(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.TrimSpace(strings.ToLower(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off", "":
			return false
		}
	case float64:
		return v != 0
	case int:
		return v != 0
	case int64:
		return v != 0
	case nil:
		return true
	}
	return true
}

func (s *InboundService) CleanupLegacyTrojanSettings() error {
	db := database.GetDB()
	inbounds := make([]*model.Inbound, 0)
	err := db.Model(model.Inbound{}).Where("protocol = ?", model.Trojan).Find(&inbounds).Error
	if err != nil {
		return err
	}

	for _, inbound := range inbounds {
		oldSettings := inbound.Settings
		s.normalizeProtocolSettings(inbound)
		if inbound.Settings == oldSettings {
			continue
		}
		if err := db.Model(&model.Inbound{}).Where("id = ?", inbound.Id).Update("settings", inbound.Settings).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *InboundService) CleanupLegacyVlessSettings() error {
	db := database.GetDB()
	inbounds := make([]*model.Inbound, 0)
	err := db.Model(model.Inbound{}).Where("protocol = ?", model.VLESS).Find(&inbounds).Error
	if err != nil {
		return err
	}

	for _, inbound := range inbounds {
		oldSettings := inbound.Settings
		s.normalizeProtocolSettings(inbound)
		if inbound.Settings == oldSettings {
			continue
		}
		if err := db.Model(&model.Inbound{}).Where("id = ?", inbound.Id).Update("settings", inbound.Settings).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *InboundService) CleanupLegacySocksSettings() error {
	db := database.GetDB()
	inbounds := make([]*model.Inbound, 0)
	err := db.Model(model.Inbound{}).Where("protocol in ?", []model.Protocol{model.Socks, model.Mixed}).Find(&inbounds).Error
	if err != nil {
		return err
	}

	for _, inbound := range inbounds {
		oldSettings := inbound.Settings
		s.normalizeProtocolSettings(inbound)
		if inbound.Settings == oldSettings {
			continue
		}
		if err := db.Model(&model.Inbound{}).Where("id = ?", inbound.Id).Update("settings", inbound.Settings).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *InboundService) GetInbounds(userId int) ([]*model.Inbound, error) {
	db := database.GetDB()
	var inbounds []*model.Inbound
	err := db.Model(model.Inbound{}).Where("user_id = ?", userId).Find(&inbounds).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	return inbounds, nil
}

func (s *InboundService) GetAllInbounds() ([]*model.Inbound, error) {
	db := database.GetDB()
	var inbounds []*model.Inbound
	err := db.Model(model.Inbound{}).Find(&inbounds).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	return inbounds, nil
}

func (s *InboundService) checkPortExist(port int, ignoreId int) (bool, error) {
	db := database.GetDB()
	db = db.Model(model.Inbound{}).Where("port = ?", port)
	if ignoreId > 0 {
		db = db.Where("id != ?", ignoreId)
	}
	var count int64
	err := db.Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *InboundService) AddInbound(inbound *model.Inbound) error {
	s.normalizeLimit(inbound)
	s.normalizeProtocolSettings(inbound)
	exist, err := s.checkPortExist(inbound.Port, 0)
	if err != nil {
		return err
	}
	if exist {
		return common.NewError("端口已存在:", inbound.Port)
	}
	db := database.GetDB()
	return db.Save(inbound).Error
}

func (s *InboundService) AddInbounds(inbounds []*model.Inbound) error {
	for _, inbound := range inbounds {
		s.normalizeLimit(inbound)
		s.normalizeProtocolSettings(inbound)
		exist, err := s.checkPortExist(inbound.Port, 0)
		if err != nil {
			return err
		}
		if exist {
			return common.NewError("端口已存在:", inbound.Port)
		}
	}

	db := database.GetDB()
	tx := db.Begin()
	var err error
	defer func() {
		if err == nil {
			tx.Commit()
		} else {
			tx.Rollback()
		}
	}()

	for _, inbound := range inbounds {
		err = tx.Save(inbound).Error
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *InboundService) DelInbound(id int) error {
	db := database.GetDB()
	return db.Delete(model.Inbound{}, id).Error
}

func (s *InboundService) GetInbound(id int) (*model.Inbound, error) {
	db := database.GetDB()
	inbound := &model.Inbound{}
	err := db.Model(model.Inbound{}).First(inbound, id).Error
	if err != nil {
		return nil, err
	}
	return inbound, nil
}

func (s *InboundService) UpdateInbound(inbound *model.Inbound) error {
	s.normalizeLimit(inbound)
	s.normalizeProtocolSettings(inbound)
	exist, err := s.checkPortExist(inbound.Port, inbound.Id)
	if err != nil {
		return err
	}
	if exist {
		return common.NewError("端口已存在:", inbound.Port)
	}

	oldInbound, err := s.GetInbound(inbound.Id)
	if err != nil {
		return err
	}
	oldInbound.Up = inbound.Up
	oldInbound.Down = inbound.Down
	oldInbound.Total = inbound.Total
	oldInbound.Remark = inbound.Remark
	oldInbound.Enable = inbound.Enable
	oldInbound.ExpiryTime = inbound.ExpiryTime
	oldInbound.IPLimit = inbound.IPLimit
	oldInbound.IPTimeout = inbound.IPTimeout
	oldInbound.PortRate = inbound.PortRate
	oldInbound.IPRate = inbound.IPRate
	oldInbound.Listen = inbound.Listen
	oldInbound.Port = inbound.Port
	oldInbound.Protocol = inbound.Protocol
	oldInbound.Settings = inbound.Settings
	oldInbound.StreamSettings = inbound.StreamSettings
	oldInbound.Sniffing = inbound.Sniffing
	oldInbound.Tag = fmt.Sprintf("inbound-%v", inbound.Port)

	db := database.GetDB()
	return db.Save(oldInbound).Error
}

func (s *InboundService) AddTraffic(traffics []*xray.Traffic) (err error) {
	if len(traffics) == 0 {
		return nil
	}
	db := database.GetDB()
	db = db.Model(model.Inbound{})
	tx := db.Begin()
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()
	for _, traffic := range traffics {
		if traffic.IsInbound {
			err = tx.Where("tag = ?", traffic.Tag).
				UpdateColumn("up", gorm.Expr("up + ?", traffic.Up)).
				UpdateColumn("down", gorm.Expr("down + ?", traffic.Down)).
				Error
			if err != nil {
				return
			}
		}
	}
	return
}

func (s *InboundService) DisableInvalidInbounds() (int64, error) {
	db := database.GetDB()
	now := time.Now().Unix() * 1000
	result := db.Model(model.Inbound{}).
		Where("((total > 0 and up + down >= total) or (expiry_time > 0 and expiry_time <= ?)) and enable = ?", now, true).
		Update("enable", false)
	err := result.Error
	count := result.RowsAffected
	return count, err
}
