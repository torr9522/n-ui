package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"x-ui/database"
	"x-ui/database/model"
	"x-ui/util/common"
	"x-ui/xray"
)

const (
	xrayAccessLogPath      = "/var/log/xray/access.log"
	xrayAccessIPSummary    = "/var/log/xray/access-ip-summary.log"
	xrayAccessIPStatePath  = "/var/lib/x-ui/access-ip-state.json"
	xrayAccessTimeLayout   = "2006/01/02 15:04:05.000000"
	xrayInternalAccessMark = "api -> api"
	accessLogEnabledKey    = "accessLogEnabled"
	aggregationEnabledKey  = "accessIPAggregationEnabled"
)

var accessIPMu sync.Mutex
var inboundTagPortPattern = regexp.MustCompile(`\[inbound-(\d+)`)
var accessIPEventPattern = regexp.MustCompile(`(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}\.\d+)\s+from\s+(\S+)\s+accepted\s+.*?\[inbound-(\d+)\]`)

type AccessIPService struct{}

type AccessIPConfig struct {
	AccessLogEnabled   bool `json:"accessLogEnabled" form:"accessLogEnabled"`
	AggregationEnabled bool `json:"aggregationEnabled" form:"aggregationEnabled"`
}

type accessIPState struct {
	Offset int64 `json:"offset"`
}

type accessIPEvent struct {
	SourceIP string
	Port     int
	SeenAt   int64
}

type xrayLogConfig struct {
	Access   string `json:"access,omitempty"`
	Error    string `json:"error,omitempty"`
	LogLevel string `json:"loglevel,omitempty"`
}

func (s *AccessIPService) ProcessAccessLog() error {
	enabled, err := s.IsAggregationEnabled()
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}

	accessIPMu.Lock()
	defer accessIPMu.Unlock()

	state, err := s.loadState()
	if err != nil {
		return err
	}

	file, err := os.Open(xrayAccessLogPath)
	if err != nil {
		if os.IsNotExist(err) {
			return s.dumpSummary()
		}
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() < state.Offset {
		state.Offset = 0
	}

	if _, err = file.Seek(state.Offset, io.SeekStart); err != nil {
		return err
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		event, ok := parseAccessIPLine(scanner.Text())
		if !ok {
			continue
		}
		if err := s.upsertRecord(event); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	offset, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	state.Offset = offset
	if err := s.saveState(state); err != nil {
		return err
	}

	return s.dumpSummary()
}

func (s *AccessIPService) GetConfig() (*AccessIPConfig, error) {
	accessLogEnabled, err := s.getBoolSetting(accessLogEnabledKey, true)
	if err != nil {
		return nil, err
	}
	aggregationEnabled, err := s.getBoolSetting(aggregationEnabledKey, true)
	if err != nil {
		return nil, err
	}
	return &AccessIPConfig{
		AccessLogEnabled:   accessLogEnabled,
		AggregationEnabled: aggregationEnabled,
	}, nil
}

func (s *AccessIPService) UpdateConfig(config *AccessIPConfig) (bool, error) {
	current, err := s.GetConfig()
	if err != nil {
		return false, err
	}

	settingService := SettingService{}
	if err := settingService.saveSetting(accessLogEnabledKey, strconv.FormatBool(config.AccessLogEnabled)); err != nil {
		return false, err
	}
	if err := settingService.saveSetting(aggregationEnabledKey, strconv.FormatBool(config.AggregationEnabled)); err != nil {
		return false, err
	}

	if current.AggregationEnabled && !config.AggregationEnabled {
		if err := s.skipCurrentAccessLog(); err != nil {
			return false, err
		}
	}

	return current.AccessLogEnabled != config.AccessLogEnabled, nil
}

func (s *AccessIPService) IsAccessLogEnabled() (bool, error) {
	return s.getBoolSetting(accessLogEnabledKey, true)
}

func (s *AccessIPService) IsAggregationEnabled() (bool, error) {
	return s.getBoolSetting(aggregationEnabledKey, true)
}

func (s *AccessIPService) GetAccessIPRecords() ([]*model.AccessIPRecord, error) {
	if err := s.ProcessAccessLog(); err != nil {
		return nil, err
	}
	db := database.GetDB()
	records := make([]*model.AccessIPRecord, 0)
	err := db.Order("last_seen desc").Find(&records).Error
	return records, err
}

func (s *AccessIPService) ClearAccessIPRecords() error {
	accessIPMu.Lock()
	defer accessIPMu.Unlock()

	db := database.GetDB()
	if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.AccessIPRecord{}).Error; err != nil {
		return err
	}

	state := &accessIPState{}
	if info, err := os.Stat(xrayAccessLogPath); err == nil {
		state.Offset = info.Size()
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := s.saveState(state); err != nil {
		return err
	}
	return s.dumpSummary()
}

func (s *AccessIPService) ApplyAccessLogSetting(config *xray.Config) error {
	enabled, err := s.IsAccessLogEnabled()
	if err != nil {
		return err
	}

	logCfg := &xrayLogConfig{}
	raw := strings.TrimSpace(string(config.LogConfig))
	if raw != "" && raw != "null" {
		if err := json.Unmarshal(config.LogConfig, logCfg); err != nil {
			return common.NewErrorf("parse xray log config failed: %v", err)
		}
	}

	if enabled {
		if logCfg.Access == "" {
			logCfg.Access = xrayAccessLogPath
		}
		if logCfg.Error == "" {
			logCfg.Error = "/var/log/xray/error.log"
		}
		if logCfg.LogLevel == "" {
			logCfg.LogLevel = "warning"
		}
	} else {
		logCfg.Access = ""
	}

	if logCfg.Access == "" && logCfg.Error == "" && logCfg.LogLevel == "" {
		config.LogConfig = nil
		return nil
	}

	data, err := json.Marshal(logCfg)
	if err != nil {
		return err
	}
	config.LogConfig = data
	return nil
}

func (s *AccessIPService) upsertRecord(event *accessIPEvent) error {
	db := database.GetDB()
	record := &model.AccessIPRecord{}
	err := db.Where("source_ip = ? AND last_port = ?", event.SourceIP, event.Port).First(record).Error
	if database.IsNotFound(err) {
		record.SourceIP = event.SourceIP
		record.LastPort = event.Port
		record.HitCount = 1
		record.FirstSeen = event.SeenAt
		record.LastSeen = event.SeenAt
		return db.Create(record).Error
	}
	if err != nil {
		return err
	}

	record.LastPort = event.Port
	record.HitCount++
	if record.FirstSeen == 0 || event.SeenAt < record.FirstSeen {
		record.FirstSeen = event.SeenAt
	}
	if event.SeenAt >= record.LastSeen {
		record.LastSeen = event.SeenAt
	}
	return db.Save(record).Error
}

func (s *AccessIPService) dumpSummary() error {
	db := database.GetDB()
	records := make([]*model.AccessIPRecord, 0)
	err := db.Order("last_seen desc").Find(&records).Error
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(xrayAccessIPSummary), 0755); err != nil {
		return err
	}
	tmpPath := xrayAccessIPSummary + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	for _, record := range records {
		firstSeen := formatUnix(record.FirstSeen)
		lastSeen := formatUnix(record.LastSeen)
		_, err = fmt.Fprintf(
			file,
			"count=%d ip=%s inbound_port=%d first_seen=%s last_seen=%s\n",
			record.HitCount,
			record.SourceIP,
			record.LastPort,
			firstSeen,
			lastSeen,
		)
		if err != nil {
			file.Close()
			return err
		}
	}

	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, xrayAccessIPSummary)
}

func (s *AccessIPService) loadState() (*accessIPState, error) {
	state := &accessIPState{}
	data, err := os.ReadFile(xrayAccessIPStatePath)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(data, state); err != nil {
		return nil, err
	}
	return state, nil
}

func (s *AccessIPService) saveState(state *accessIPState) error {
	if err := os.MkdirAll(filepath.Dir(xrayAccessIPStatePath), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	tmpPath := xrayAccessIPStatePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, xrayAccessIPStatePath)
}

func (s *AccessIPService) getBoolSetting(key string, defaultValue bool) (bool, error) {
	settingService := SettingService{}
	setting, err := settingService.getSetting(key)
	if database.IsNotFound(err) {
		return defaultValue, nil
	}
	if err != nil {
		return false, err
	}

	value := strings.TrimSpace(strings.ToLower(setting.Value))
	if value == "" {
		return defaultValue, nil
	}

	switch value {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return false, err
		}
		return parsed, nil
	}
}

func (s *AccessIPService) skipCurrentAccessLog() error {
	accessIPMu.Lock()
	defer accessIPMu.Unlock()

	state := &accessIPState{}
	if info, err := os.Stat(xrayAccessLogPath); err == nil {
		state.Offset = info.Size()
	} else if !os.IsNotExist(err) {
		return err
	}
	return s.saveState(state)
}

func parseAccessIPLine(line string) (*accessIPEvent, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, false
	}

	matches := accessIPEventPattern.FindAllStringSubmatch(line, -1)
	if len(matches) == 0 {
		return nil, false
	}

	match := matches[len(matches)-1]
	if len(match) != 4 {
		return nil, false
	}

	seenAt, err := time.ParseInLocation(xrayAccessTimeLayout, match[1], time.Local)
	if err != nil {
		return nil, false
	}

	sourceIP, _, err := splitAccessEndpoint(match[2])
	if err != nil {
		return nil, false
	}
	inboundPort, err := strconv.Atoi(match[3])
	if err != nil || inboundPort <= 0 {
		return nil, false
	}
	if isLoopbackAddress(sourceIP) {
		return nil, false
	}

	return &accessIPEvent{
		SourceIP: sourceIP,
		Port:     inboundPort,
		SeenAt:   seenAt.Unix(),
	}, true
}

func splitAccessEndpoint(endpoint string) (string, int, error) {
	endpoint = strings.TrimSpace(endpoint)
	host, portStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		idx := strings.LastIndex(endpoint, ":")
		if idx <= 0 || idx >= len(endpoint)-1 {
			return "", 0, err
		}
		host = endpoint[:idx]
		portStr = endpoint[idx+1:]
	}
	host = normalizeSourceHost(host)
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	if host == "" {
		return "", 0, common.NewError("empty source ip")
	}
	return host, port, nil
}

func normalizeSourceHost(host string) string {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	for {
		parts := strings.SplitN(host, ":", 2)
		if len(parts) != 2 {
			break
		}
		prefix := strings.ToLower(strings.TrimSpace(parts[0]))
		switch prefix {
		case "tcp", "udp", "unix":
			host = strings.TrimSpace(parts[1])
			continue
		}
		break
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	if strings.Count(host, ":") > 1 {
		return host
	}
	return host
}

func isLoopbackAddress(host string) bool {
	host = normalizeSourceHost(host)
	if host == "" {
		return true
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func formatUnix(ts int64) string {
	if ts <= 0 {
		return "-"
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}
