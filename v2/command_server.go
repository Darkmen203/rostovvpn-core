package v2

import (
	"sync"

	"github.com/sagernet/sing-box/experimental/clashapi"
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
	if svc.clashServer != nil {
		if srv, ok := svc.clashServer.(*clashapi.Server); ok {
			srv.SetModeUpdateHook(s.modeUpdate)
		}
	}
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
