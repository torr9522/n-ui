package job

import (
	"x-ui/logger"
	"x-ui/web/service"
)

type AccessIPJob struct {
	accessIPService service.AccessIPService
}

func NewAccessIPJob() *AccessIPJob {
	return new(AccessIPJob)
}

func (j *AccessIPJob) Run() {
	err := j.accessIPService.ProcessAccessLog()
	if err != nil {
		logger.Warning("process xray access ip log failed:", err)
	}
}
