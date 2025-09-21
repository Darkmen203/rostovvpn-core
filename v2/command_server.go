package v2

import (
	"fmt"
	"sync"

	"time"

	"github.com/sagernet/sing-box/experimental/clashapi"
	"github.com/sagernet/sing-box/log"
)

// Минимальный сервер: только два события (URL-test обновления и смена Clash-режима)
// и привязка их к живому экземпляру CoreService.
type CommandServer struct {
	mu            sync.Mutex
	service       *CoreService
	urlTestUpdate chan struct{}
	modeUpdate    chan struct{}
	closed        bool
}

func NewCommandServer(_ int32) *CommandServer {
	return &CommandServer{
		urlTestUpdate: make(chan struct{}, 1),
		modeUpdate:    make(chan struct{}, 1),
	}
}

// Вызывается после старта/рестарта ядра: подвешиваем хуки к текущему CoreService.
func (s *CommandServer) SetService(svc *CoreService) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.service = svc
	if svc == nil {
		return
	}
	// URL-test: отдать канал обновлений истории
	if svc.urlHistory != nil {
		svc.urlHistory.SetHook(s.urlTestUpdate)
	}
	// Clash mode: если Clash API поднят — отдать канал для оповещения
	fmt.Println("[SetService] !!! svc.clashServer=\n\n\n", svc.clashServer, "\n [SetService]")
	if svc.clashServer == nil {
		log.Info("Clash API not present — skip hooks")
		return
	}

	// ЕСЛИ оставишь включаемый Clash: только тогда вешай хук,
	// и в v1.12.8 он принимает КАНАЛ:
	go func(srv *clashapi.Server) {
		time.Sleep(200 * time.Millisecond)
		defer func() {
			if r := recover(); r != nil {
				log.Warn("Skip Clash hook setup: ", r)
			}
		}()
		srv.SetModeUpdateHook(s.modeUpdate)
		log.Info("Clash mode hook installed")
	}(svc.clashServer)
}

// Заглушки под старый интерфейс (ничего не слушаем, просто OK)
func (s *CommandServer) Start() error { return nil }

func (s *CommandServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	// каналы не закрываем специально, чтобы не гонять гонки при перевешивании хуков
	return nil
}
