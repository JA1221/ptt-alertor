package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/channels/telegram"
	"github.com/Ptt-Alertor/ptt-alertor/command"
	"github.com/Ptt-Alertor/ptt-alertor/config"
	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/jobs"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/keyword"
	pttweb "github.com/Ptt-Alertor/ptt-alertor/ptt/web"
	"github.com/Ptt-Alertor/ptt-alertor/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.WithError(err).Fatal("Config Invalid")
	}
	if len(cfg.AllowedIDs) == 0 {
		log.Warn("TELEGRAM_ALLOWED_IDS Is Empty, Nobody Can Use the Bot. Send Any Message to the Bot to Get Your ID")
	}
	accounts := make([]string, 0, len(cfg.AllowedIDs))
	for _, id := range cfg.AllowedIDs {
		accounts = append(accounts, strconv.FormatInt(id, 10))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !waitRedis(ctx) {
		return
	}
	// checking boards follow subscriptions of allowed users
	if err := keyword.Sync(accounts); err != nil {
		log.WithError(err).Fatal("Sync Keyword Index Failed")
	}
	log.WithField("boards", board.List()).Info("Checking Boards")

	tracker := health.New()
	handler := command.NewHandler(accounts, tracker)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           (&web.Server{Health: tracker, Accounts: accounts}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.WithField("addr", cfg.HTTPAddr).Info("HTTP Server Start")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("HTTP Server Failed")
		}
	}()

	bot := connectTelegram(ctx, cfg, handler, tracker)
	if bot == nil {
		return
	}
	log.Info("Telegram Authorized on " + bot.UserName())

	var wg sync.WaitGroup
	run := func(f func(ctx context.Context)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f(ctx)
		}()
	}
	run(bot.Run)
	run((&jobs.BoardChecker{
		Fetch:    board.Fetch,
		Notifier: bot,
		Health:   tracker,
		Interval: 1 * time.Second,
		Cycle:    15 * time.Second,
	}).Run)
	run((&jobs.CommentChecker{
		FetchArticle: pttweb.FetchArticle,
		Notifier:     bot,
		Health:       tracker,
		Interval:     1 * time.Second,
		Cycle:        time.Minute,
	}).Run)
	run((&jobs.PTTMonitor{
		Health: tracker,
		Client: &http.Client{Timeout: 30 * time.Second},
	}).Run)
	run((&jobs.Alerter{
		Health:   tracker,
		Notifier: bot,
		Owners:   cfg.AllowedIDs,
		Interval: 30 * time.Second,
	}).Run)

	<-ctx.Done()
	log.Info("Shutting Down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	// telegram long polling may not return until its timeout
	select {
	case <-done:
	case <-shutdownCtx.Done():
		log.Warn("Jobs Not Stopped in Time")
	}
	log.Info("Bye")
}

// waitRedis waits until redis is reachable, false when ctx is done first
func waitRedis(ctx context.Context) bool {
	for {
		err := connections.PingRedis()
		if err == nil {
			return true
		}
		log.WithError(err).WithField("addr", connections.RedisAddr()).Warn("Redis Unreachable, Retry")
		select {
		case <-ctx.Done():
			return false
		case <-time.After(3 * time.Second):
		}
	}
}

// connectTelegram retries until connected, nil when ctx is done first
func connectTelegram(ctx context.Context, cfg config.Config, handler *command.Handler, tracker *health.Tracker) *telegram.Bot {
	for {
		bot, err := telegram.New(cfg.TelegramToken, cfg.AllowedIDs, handler.Handle, tracker)
		if err == nil {
			return bot
		}
		if strings.Contains(err.Error(), "Unauthorized") {
			log.WithError(err).Fatal("TELEGRAM_TOKEN Invalid")
		}
		tracker.TelegramPolled(err)
		log.WithError(err).Error("Telegram Connect Failed, Retry")
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(10 * time.Second):
		}
	}
}
