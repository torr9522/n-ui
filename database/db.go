package database

import (
	"encoding/json"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io/fs"
	"os"
	"path"
	"strings"
	"x-ui/config"
	"x-ui/database/model"
)

var db *gorm.DB

func initUser() error {
	err := db.AutoMigrate(&model.User{})
	if err != nil {
		return err
	}
	var count int64
	err = db.Model(&model.User{}).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		user := &model.User{
			Username: "admin",
			Password: "admin",
		}
		return db.Create(user).Error
	}
	return nil
}

func initInbound() error {
	err := db.AutoMigrate(&model.Inbound{})
	if err != nil {
		return err
	}
	if err = db.Exec("UPDATE inbounds SET ip_limit = 0 WHERE ip_limit IS NULL").Error; err != nil {
		return err
	}
	if err = db.Exec("UPDATE inbounds SET ip_timeout = 5 WHERE ip_timeout IS NULL OR ip_timeout <= 0").Error; err != nil {
		return err
	}
	if err = db.Exec("UPDATE inbounds SET port_rate = '' WHERE port_rate IS NULL").Error; err != nil {
		return err
	}
	if err = db.Exec("UPDATE inbounds SET ip_rate = '' WHERE ip_rate IS NULL").Error; err != nil {
		return err
	}
	if err = cleanupLegacyTrojanSettings(); err != nil {
		return err
	}
	if err = cleanupLegacyVlessSettings(); err != nil {
		return err
	}
	return nil
}

func cleanupLegacyTrojanSettings() error {
	type trojanInboundSettings struct {
		Clients   []map[string]interface{} `json:"clients"`
		Fallbacks []map[string]interface{} `json:"fallbacks"`
	}

	inbounds := make([]*model.Inbound, 0)
	if err := db.Model(model.Inbound{}).Where("protocol = ?", model.Trojan).Find(&inbounds).Error; err != nil {
		return err
	}

	for _, inbound := range inbounds {
		settings := strings.TrimSpace(inbound.Settings)
		if settings == "" {
			continue
		}
		var trojan trojanInboundSettings
		if err := json.Unmarshal([]byte(settings), &trojan); err != nil {
			continue
		}
		changed := false
		for _, client := range trojan.Clients {
			if _, ok := client["flow"]; ok {
				delete(client, "flow")
				changed = true
			}
		}
		if !changed {
			continue
		}
		data, err := json.Marshal(trojan)
		if err != nil {
			return err
		}
		if err := db.Model(&model.Inbound{}).Where("id = ?", inbound.Id).Update("settings", string(data)).Error; err != nil {
			return err
		}
	}
	return nil
}

func cleanupLegacyVlessSettings() error {
	type vlessInboundSettings struct {
		Clients    []map[string]interface{} `json:"clients"`
		Decryption string                   `json:"decryption"`
		Fallbacks  []map[string]interface{} `json:"fallbacks"`
	}

	inbounds := make([]*model.Inbound, 0)
	if err := db.Model(model.Inbound{}).Where("protocol = ?", model.VLESS).Find(&inbounds).Error; err != nil {
		return err
	}

	for _, inbound := range inbounds {
		settings := strings.TrimSpace(inbound.Settings)
		if settings == "" {
			continue
		}
		var vless vlessInboundSettings
		if err := json.Unmarshal([]byte(settings), &vless); err != nil {
			continue
		}
		changed := false
		for _, client := range vless.Clients {
			if flow, ok := client["flow"].(string); ok {
				if flow == "xtls-rprx-direct" || flow == "xtls-rprx-origin" {
					client["flow"] = ""
					changed = true
				}
			}
		}
		if !changed {
			continue
		}
		data, err := json.Marshal(vless)
		if err != nil {
			return err
		}
		if err := db.Model(&model.Inbound{}).Where("id = ?", inbound.Id).Update("settings", string(data)).Error; err != nil {
			return err
		}
	}
	return nil
}

func initSetting() error {
	return db.AutoMigrate(&model.Setting{})
}

func initAccessIPRecord() error {
	return db.AutoMigrate(&model.AccessIPRecord{})
}

func InitDB(dbPath string) error {
	dir := path.Dir(dbPath)
	err := os.MkdirAll(dir, fs.ModeDir)
	if err != nil {
		return err
	}

	var gormLogger logger.Interface

	if config.IsDebug() {
		gormLogger = logger.Default
	} else {
		gormLogger = logger.Discard
	}

	c := &gorm.Config{
		Logger: gormLogger,
	}
	db, err = gorm.Open(sqlite.Open(dbPath), c)
	if err != nil {
		return err
	}

	err = initUser()
	if err != nil {
		return err
	}
	err = initInbound()
	if err != nil {
		return err
	}
	err = initSetting()
	if err != nil {
		return err
	}
	err = initAccessIPRecord()
	if err != nil {
		return err
	}

	return nil
}

func GetDB() *gorm.DB {
	return db
}

func IsNotFound(err error) bool {
	return err == gorm.ErrRecordNotFound
}
