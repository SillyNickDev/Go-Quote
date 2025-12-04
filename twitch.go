package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	twitch "github.com/gempir/go-twitch-irc/v2"
)

// TwitchBot wraps the IRC client with reconnection and command handling logic.
type TwitchBot struct {
	client        *twitch.Client
	handler       *CommandHandler
	channel       string
	minRetryDelay time.Duration
	maxRetryDelay time.Duration

	random     *rand.Rand
	randomMu   sync.Mutex
	retryMu    sync.Mutex
	retryDelay time.Duration
}

// NewTwitchBot creates and configures a TwitchBot for the given IRC client, command handler, and channel.
// It initializes default retry delays and registers client event handlers for connect, reconnect, notice, and private messages so incoming messages are passed to the CommandHandler.
func NewTwitchBot(client *twitch.Client, handler *CommandHandler, channel string) *TwitchBot {
	bot := &TwitchBot{
		client:        client,
		handler:       handler,
		channel:       channel,
		minRetryDelay: time.Second,
		maxRetryDelay: 30 * time.Second,
		random:        rand.New(rand.NewSource(time.Now().UnixNano())),
		retryDelay:    time.Second,
	}

	client.OnConnect(func() {
		log.Printf("Connected to Twitch. Joining #%s", channel)
		client.Join(channel)
		bot.resetRetryBackoff()
	})
	client.OnReconnectMessage(func(message twitch.ReconnectMessage) {
		log.Printf("Twitch requested reconnect for channel #%s", channel)
		go func() {
			if err := client.Disconnect(); err != nil && !errors.Is(err, twitch.ErrClientDisconnected) {
				log.Printf("Error disconnecting Twitch client: %v", err)
			}
		}()
	})
	client.OnNoticeMessage(func(message twitch.NoticeMessage) {
		log.Printf("NOTICE [%s]: %s", message.Channel, message.Message)
	})
	client.OnPrivateMessage(func(message twitch.PrivateMessage) {
		go bot.handleMessage(message)
	})

	return bot
}

func (b *TwitchBot) handleMessage(message twitch.PrivateMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	responses := b.handler.Handle(ctx, message.Message, message.User.Name, isModerator(message.User))
	for _, response := range responses {
		b.client.Say(message.Channel, response)
	}
}

// Run connects the bot to Twitch and keeps trying until the context is canceled.
func (b *TwitchBot) Run(ctx context.Context) error {
	for {
		errCh := make(chan error, 1)
		go func() {
			errCh <- b.client.Connect()
		}()

		select {
		case <-ctx.Done():
			_ = b.client.Disconnect()
			return ctx.Err()
		case err := <-errCh:
			if ctx.Err() != nil {
				_ = b.client.Disconnect()
				return ctx.Err()
			}
			if err == nil || errors.Is(err, twitch.ErrClientDisconnected) {
				log.Printf("Twitch client disconnected, attempting to reconnect...")
			} else {
				log.Printf("Twitch connection error: %v", err)
			}
			delay := b.backoffDelay()
			log.Printf("Retrying Twitch connection in %s...", delay)
			select {
			case <-ctx.Done():
				_ = b.client.Disconnect()
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
}

func (b *TwitchBot) resetRetryBackoff() {
	b.retryMu.Lock()
	defer b.retryMu.Unlock()
	if b.minRetryDelay <= 0 {
		b.minRetryDelay = time.Second
	}
	b.retryDelay = b.minRetryDelay
}

func (b *TwitchBot) backoffDelay() time.Duration {
	b.retryMu.Lock()
	delay := b.retryDelay
	if delay < b.minRetryDelay {
		delay = b.minRetryDelay
	}
	if delay > b.maxRetryDelay {
		delay = b.maxRetryDelay
	}
	next := delay * 2
	if next > b.maxRetryDelay {
		next = b.maxRetryDelay
	}
	b.retryDelay = next
	b.retryMu.Unlock()

	jitter := 0.5 + b.randomFloat64()
	jittered := time.Duration(float64(delay) * jitter)
	if jittered < b.minRetryDelay {
		jittered = b.minRetryDelay
	}
	if jittered > b.maxRetryDelay {
		jittered = b.maxRetryDelay
	}
	return jittered
}

func (b *TwitchBot) randomFloat64() float64 {
	b.randomMu.Lock()
	defer b.randomMu.Unlock()
	if b.random == nil {
		return rand.Float64()
	}
	return b.random.Float64()
}

// configureTwitchClient creates and returns a Twitch IRC client configured for Twitch chat.
// It enables TLS, applies the library's default capabilities, sets the Twitch IRC address,
// clears any setup command, and assigns the default rate limiter so the client is ready to connect.
func configureTwitchClient(user, oauth string) *twitch.Client {
	client := twitch.NewClient(user, oauth)
	client.TLS = true
	client.Capabilities = twitch.DefaultCapabilities
	client.IrcAddress = "irc.chat.twitch.tv:6697"
	client.SetupCmd = ""
	client.SetRateLimiter(twitch.CreateDefaultRateLimiter())
	return client
}

// validateTwitchConfig verifies that the Twitch user, OAuth token, and channel are provided.
// It returns an error describing the missing credentials if any of the three values are empty.
func validateTwitchConfig(user, oauth, channel string) error {
	if user == "" || oauth == "" || channel == "" {
		return fmt.Errorf("twitch credentials (user, oauth, channel) must be provided in twitch mode")
	}
	return nil
}

func isModerator(user twitch.User) bool {
	if user.Badges == nil {
		return false
	}
	if _, ok := user.Badges["broadcaster"]; ok {
		return true
	}
	if _, ok := user.Badges["moderator"]; ok {
		return true
	}
	return false
}
