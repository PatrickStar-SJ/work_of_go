package async

import (
	"context"
	"gitee.com/geekbang/basic-go/webook/pkg/logger"
	"gitee.com/geekbang/basic-go/webook/sms/domain"
	"gitee.com/geekbang/basic-go/webook/sms/repository"
	"gitee.com/geekbang/basic-go/webook/sms/service"
	"time"
)

type Service struct {
	svc  service.Service
	repo repository.AsyncSmsRepository
	l    logger.LoggerV1
}

func NewService(svc service.Service,
	repo repository.AsyncSmsRepository,
	l logger.LoggerV1) *Service {
	res := &Service{
		svc:  svc,
		repo: repo,
		l:    l,
	}
	go func() {
		res.StartAsyncCycle()
	}()
	return res
}

func (s *Service) StartAsyncCycle() {
	for {
		s.AsyncSend()
	}
}

func (s *Service) AsyncSend() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	as, err := s.repo.PreemptWaitingSMS(ctx)
	cancel()
	switch err {
	case nil:
		ctx, cancel = context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err = s.svc.Send(ctx, as.TplId, as.Args, as.Numbers...)
		if err != nil {
			s.l.Error("Failed to execute async SMS sending",
				logger.Error(err),
				logger.Int64("id", as.Id))
		}
		res := err == nil
		err = s.repo.ReportScheduleResult(ctx, as.Id, res)
		if err != nil {
			s.l.Error("Successfully executed async SMS sending, but failed to update database",
				logger.Error(err),
				logger.Bool("res", res),
				logger.Int64("id", as.Id))
		}
	case repository.ErrWaitingSMSNotFound:
		time.Sleep(time.Second)
	default:
		s.l.Error("Failed to preempt async SMS sending task",
			logger.Error(err))
		time.Sleep(time.Second)
	}
}

func (s *Service) Send(ctx context.Context, tplId string, args []string, numbers ...string) error {
	if s.needAsync() {
		err := s.repo.Add(ctx, domain.AsyncSms{
			TplId:   tplId,
			Args:    args,
			Numbers: numbers,
			RetryMax: 3,
		})
		return err
	}
	return s.svc.Send(ctx, tplId, args, numbers...)
}

func (s *Service) needAsync() bool {
	return true
}
