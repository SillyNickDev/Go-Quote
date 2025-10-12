package main

import (
	"context"
	"errors"
	"fmt"
	"log"
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
}

func NewTwitchBot(client *twitch.Client, handler *CommandHandler, channel string) *TwitchBot {
	bot := &TwitchBot{
		client:        client,
		handler:       handler,
		channel:       channel,
		minRetryDelay: time.Second,
		maxRetryDelay: 30 * time.Second,
	}

	client.OnConnect(func() {
		log.Printf("Connected to Twitch. Joining #%s", channel)
		client.Join(channel)
	})
	client.OnReconnectMessage(func(message twitch.ReconnectMessage) {
		log.Printf("Twitch requested reconnect for channel #%s", channel)
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

	responses := b.handler.Handle(ctx, message.Message, message.User.Name)
	for _, response := range responses {
		b.client.Say(message.Channel, response)
	}
}

// Run connects the bot to Twitch and keeps trying until the context is canceled.
func (b *TwitchBot) Run(ctx context.Context) error {
	retryDelay := b.minRetryDelay
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
			if err == nil || errors.Is(err, twitch.ErrClientDisconnected) {
				return nil
			}
			log.Printf("Twitch connection error: %v", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelay):
			}
			retryDelay *= 2
			if retryDelay > b.maxRetryDelay {
				retryDelay = b.maxRetryDelay
			}
			log.Printf("Retrying Twitch connection in %s...", retryDelay)
		}
	}
}

func configureTwitchClient(user, oauth string) *twitch.Client {
	client := twitch.NewClient(user, oauth)
	client.TLS = true
	client.Capabilities = twitch.DefaultCapabilities
	client.IrcAddress = "irc.chat.twitch.tv:6697"
	client.SetupCmd = ""
	client.SetRateLimiter(twitch.CreateDefaultRateLimiter())
	return client
}

func validateTwitchConfig(user, oauth, channel string) error {
	if user == "" || oauth == "" || channel == "" {
		return fmt.Errorf("twitch credentials (user, oauth, channel) must be provided in twitch mode")
	}
	return nil
}
