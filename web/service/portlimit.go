package service

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"x-ui/logger"
)

const (
	portLimitSyncScript  = "/usr/local/bin/xui-portlimit-sync.sh"
	portLimitSyncService = "xui-portlimit-sync.service"
	portLimitTable       = "xui_auto_portlimit"
)

type PortLimitService struct{}

func (s *PortLimitService) SyncNow() {
	go func() {
		output, err := s.syncWithOutput()
		if err != nil {
			logger.Warning("run port-limit sync script failed:", err, ", output:", output)
			return
		}
		logger.Debug("run port-limit sync script done:", output)
	}()
}

func (s *PortLimitService) SyncNowBlocking() (string, error) {
	return s.syncWithOutput()
}

func (s *PortLimitService) GetRecentLogs(lines int) (string, error) {
	if lines <= 0 {
		lines = 80
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "journalctl", "-u", portLimitSyncService, "-n", stringInt(lines), "--no-pager", "-o", "short-iso")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func (s *PortLimitService) GetRuleStatus() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "nft", "list", "table", "inet", portLimitTable)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), err
}

func (s *PortLimitService) BuildDiagnostics() (string, error) {
	var sb strings.Builder
	sb.WriteString("== xui-portlimit diagnostics ==\n")
	sb.WriteString("time: " + time.Now().Format(time.RFC3339) + "\n\n")

	syncOut, syncErr := s.syncWithOutput()
	sb.WriteString("[sync]\n")
	sb.WriteString(syncOut + "\n")
	if syncErr != nil {
		sb.WriteString("sync_error: " + syncErr.Error() + "\n")
	}
	sb.WriteString("\n")

	ruleOut, ruleErr := s.GetRuleStatus()
	sb.WriteString("[nft table]\n")
	sb.WriteString(ruleOut + "\n")
	if ruleErr != nil {
		sb.WriteString("nft_error: " + ruleErr.Error() + "\n")
	}
	sb.WriteString("\n")

	logOut, logErr := s.GetRecentLogs(120)
	sb.WriteString("[recent logs]\n")
	sb.WriteString(logOut + "\n")
	if logErr != nil {
		sb.WriteString("log_error: " + logErr.Error() + "\n")
	}
	sb.WriteString("\n")

	serviceOut, serviceErr := runCmd(12*time.Second, "systemctl", "status", portLimitSyncService, "--no-pager")
	sb.WriteString("[service status]\n")
	sb.WriteString(serviceOut + "\n")
	if serviceErr != nil {
		sb.WriteString("service_error: " + serviceErr.Error() + "\n")
	}
	return sb.String(), nil
}

func (s *PortLimitService) syncWithOutput() (string, error) {
	return runCmd(20*time.Second, portLimitSyncScript)
}

func runCmd(timeout time.Duration, cmdName string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, cmdName, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return out.String(), fmt.Errorf("%s %v failed: %w", cmdName, args, err)
	}
	return out.String(), nil
}

func stringInt(v int) string {
	return strconv.Itoa(v)
}
