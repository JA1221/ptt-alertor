package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/channels/telegram"
	"github.com/Ptt-Alertor/ptt-alertor/command"
	"github.com/Ptt-Alertor/ptt-alertor/config"
	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/health"
	"github.com/Ptt-Alertor/ptt-alertor/jobs"
	"github.com/Ptt-Alertor/ptt-alertor/models/access"
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
	// docker healthcheck, the image has no shell or curl
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(cfg.HTTPAddr))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if !waitRedis(ctx) {
		return
	}
	if err := prepare(cfg); err != nil {
		log.WithError(err).Fatal("Prepare Data Failed")
	}

	tracker := health.New()
	tokens := make(chan string, 1)
	hasToken := func() bool {
		if cfg.TelegramToken != "" {
			return true
		}
		token, _ := access.Token()
		return token != ""
	}
	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: (&web.Server{
			Health:   tracker,
			HasToken: hasToken,
			SaveToken: func(token string) (string, error) {
				name, err := telegram.Verify(token)
				if err != nil {
					return "", err
				}
				if err := access.SetToken(token); err != nil {
					return "", err
				}
				select {
				case tokens <- token:
				default:
				}
				return name, nil
			},
		}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.WithField("addr", cfg.HTTPAddr).Info("HTTP Server Start")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("HTTP Server Failed")
		}
	}()

	var wg sync.WaitGroup
	if bot := connectTelegram(ctx, cfg, tokens, tracker); bot != nil {
		start(ctx, &wg, bot, tracker)
	}

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

// prepare migrates data of older versions and rebuilds keyword index from members
func prepare(cfg config.Config) error {
	if err := access.Migrate(); err != nil {
		return err
	}
	for _, id := range cfg.AdminIDs {
		if err := access.AddAdmin(strconv.FormatInt(id, 10)); err != nil {
			return err
		}
	}
	members, err := access.Members()
	if err != nil {
		return err
	}
	if err := keyword.Rebuild(members); err != nil {
		return err
	}
	log.WithFields(log.Fields{"members": len(members), "boards": len(board.List())}).Info("Data Ready")
	return nil
}

func start(ctx context.Context, wg *sync.WaitGroup, bot *telegram.Bot, tracker *health.Tracker) {
	handler := command.NewHandler(tracker, bot)
	bot.SetHandler(handler.Handle)
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
	run((&jobs.Monitor{
		Health: tracker,
		Client: &http.Client{Timeout: 30 * time.Second},
	}).Run)
	run((&jobs.Alerter{
		Health:   tracker,
		Notifier: bot,
		Interval: 30 * time.Second,
	}).Run)
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

// connectTelegram waits for a token (env, saved or from setup page) and retries until connected,
// nil when ctx is done first
func connectTelegram(ctx context.Context, cfg config.Config, tokens <-chan string, tracker *health.Tracker) *telegram.Bot {
	token := cfg.TelegramToken
	if token == "" {
		token, _ = access.Token()
	}
	if token == "" {
		log.Warn("Telegram Not Set Up, Open http://localhost:9090 to Set Up")
		select {
		case <-ctx.Done():
			return nil
		case token = <-tokens:
		}
	}
	for {
		bot, err := telegram.New(token, nil, tracker)
		if err == nil {
			tracker.SetBot(bot.UserName())
			log.Info("Telegram Authorized on " + bot.UserName())
			return bot
		}
		tracker.TelegramPolled(err)
		if strings.Contains(err.Error(), "Unauthorized") && cfg.TelegramToken == "" {
			// e.g. token revoked by BotFather, setup page asks a new one
			log.WithError(err).Error("Telegram Token Invalid, Open http://localhost:9090 to Set Up Again")
			if err := access.SetToken(""); err != nil {
				log.WithError(err).Error("Clear Telegram Token Failed")
			}
			select {
			case <-ctx.Done():
				return nil
			case token = <-tokens:
			}
			continue
		}
		log.WithError(err).Error("Telegram Connect Failed, Retry")
		select {
		case <-ctx.Done():
			return nil
		case token = <-tokens:
		case <-time.After(30 * time.Second):
		}
	}
}

func healthcheck(addr string) int {
	port := addr[strings.LastIndex(addr, ":")+1:]
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
