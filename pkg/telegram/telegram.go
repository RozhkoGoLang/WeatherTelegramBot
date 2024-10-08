package telegram

import (
	"time"

	"projecttelegrambot/pkg/holiday"
	"projecttelegrambot/pkg/mongodb"
	"projecttelegrambot/pkg/weather"

	"git.foxminded.ua/foxstudent107249/telegrambot"
)

const (
	telegramAPI          = "https://api.telegram.org/bot"
	DefaultHelpStartInfo = `
/start   - get keyboard with flags
/help    - too same like start
/weather - get the current weather for your location
/subcsribe -   subscription to the weather report,
/unsubcsribe - unsubscription to the weather report,
/about  - get some information about me
/links   - send my(developer) links`

	DefaultLinksInfo = `
https://www.linkedin.com/in/dmytro-rozhko-bas-1c-golang-junior/
https://animated-panda-0382af.netlify.app/
	`
	CurrentParseMode = telegrambot.ModeMarkdownV2
)

type TelegramService struct {
	apiTelegram     *telegrambot.ApiTelegramBot
	apiHoliday      *holiday.ApiHoliday
	apiWeather      *weather.ApiWeather
	mongoDBSrv      *mongodb.MongoDBService
	previousCommand PreviousCommand
}

type PreviousCommand map[int]string

var infoMap = map[string]string{
	"/start":       DefaultHelpStartInfo,
	"/help":        DefaultHelpStartInfo,
	"/weather":     "Get the current weather for your location",
	"/subcsribe":   "Subscription to the weather report",
	"/unsubcsribe": "Unsubscription to the weather report",
	"/about":       "Rozhko Dmytro; Go developer; bad character; unmarried (C)",
	"/links":       DefaultLinksInfo,
}

var DefualtKeyboard = telegrambot.ReplyKeyboardMarkup{
	Keyboard: [][]telegrambot.KeyboardButton{
		{
			{Text: DefaultFlags[0]},
			{Text: DefaultFlags[1]},
			{Text: DefaultFlags[2]},
		},
		{
			{Text: DefaultFlags[3]},
			{Text: DefaultFlags[4]},
			{Text: DefaultFlags[5]},
		},
	},
	ResizeKeyboard:  true,
	OneTimeKeyboard: true,
}

var DefualtKeyboardGeolacation = telegrambot.ReplyKeyboardMarkup{
	Keyboard: [][]telegrambot.KeyboardButton{
		{
			{Text: "Give Your location", RequestLocation: true},
		},
	},
	ResizeKeyboard:  true,
	OneTimeKeyboard: true,
}

var DefaultFlags = []string{
	"🇺🇸 USA",
	"🇬🇧 UK",
	"🇨🇦 Canada",
	"🇦🇺 Australia",
	"🇮🇳 India",
	"🇺🇦 Ukraine",
}

var flagsCountryMap = map[string]string{
	DefaultFlags[0]: "US",
	DefaultFlags[1]: "GB",
	DefaultFlags[2]: "CA",
	DefaultFlags[3]: "AU",
	DefaultFlags[4]: "IN",
	DefaultFlags[5]: "UA",
}

func NewMyTelegramService(apiTelegram *telegrambot.ApiTelegramBot, apiHoliday *holiday.ApiHoliday, apiWeather *weather.ApiWeather, mongoDBSrv *mongodb.MongoDBService) *TelegramService {
	return &TelegramService{
		apiTelegram:     apiTelegram,
		apiHoliday:      apiHoliday,
		apiWeather:      apiWeather,
		mongoDBSrv:      mongoDBSrv,
		previousCommand: PreviousCommand{},
	}
}

func (c *TelegramService) CreateSendResponse(update *telegrambot.Update) error {
	command := update.Message.Text
	chatId := update.Message.Chat.ID

	c.saveCommand(chatId, command)

	switch command {
	case "/start":
		_, err := c.apiTelegram.CreateReplyKeyboard(chatId, command, DefualtKeyboard)
		return err
	case "/weather", "/subcsribe":
		_, err := c.apiTelegram.CreateReplyKeyboard(chatId, "Pls, get location", DefualtKeyboardGeolacation)
		return err
	case "/unsubcsribe":
		return c.mongoDBSrv.Unsubscribe(chatId)
	default:
		// Unknown
		if isUnknownCommand(update) {
			_, err := c.apiTelegram.CreateReplayMsg(chatId, "", CurrentParseMode)
			return err
		}
		// standart command
		if infoMap[command] != "" {
			_, err := c.apiTelegram.CreateReplayMsg(chatId, infoMap[command], CurrentParseMode)
			return err
		}
		// responce with country value
		if flagsCountryMap[command] != "" {
			return c.createReplayMsgHoliday(update)
		}
		// responce with fill Location
		if update.Message.Location != nil {
			// check previous command!
			switch c.previousCommand[chatId] {
			case "/subcsribe":
				err := c.mongoDBSrv.Subscribe(chatId, update.Message.Location.Latitude, update.Message.Location.Longitude, time.Now())
				if err == nil {
					c.apiTelegram.CreateReplayMsg(chatId, "Subscription successfully added", CurrentParseMode)
				}
				return err
			default:
				return c.createReplayMsgWeather(update)
			}
		}

	}
	return nil
}

// check subscribers in this hour and send report about currently weather
func (c *TelegramService) CheckSubscribers(done chan bool, ticker *time.Ticker) {
	for {
		select {
		case <-done:
			return
		case t := <-ticker.C:
			subscribers, err := c.mongoDBSrv.GetSubsribersByTime(t.Hour())
			if err != nil {
				c.apiTelegram.Logger.Error("Can`t create report", "Error", err)
			}
			if len(subscribers) != 0 {
				c.SendReportWeather(subscribers)
			}

		}
	}
}

// send API request and create text message with weather and sen him
func (c *TelegramService) createReplayMsgWeather(update *telegrambot.Update) error {
	chatId := update.Message.Chat.ID
	resp, err := c.apiWeather.Load(update.Message.Location.Latitude, update.Message.Location.Longitude)
	if err != nil {
		return err
	}
	geotxt, err := resp.Description()
	if err != nil {
		return err
	}
	_, err = c.apiTelegram.CreateReplayMsg(chatId, geotxt, CurrentParseMode)
	return err
}

// send API request and create text message with holidays and sen him
func (c *TelegramService) createReplayMsgHoliday(update *telegrambot.Update) error {
	command := update.Message.Text
	chatId := update.Message.Chat.ID
	text, err := c.apiHoliday.Names(flagsCountryMap[command], time.Now())
	if err != nil {
		return err
	}
	_, err = c.apiTelegram.CreateReplayMsg(chatId, text, CurrentParseMode)
	return err
}

func (c *TelegramService) SendReportWeather(subscribers []mongodb.Subscribe) {
	for _, s := range subscribers {
		var update telegrambot.Update
		var location telegrambot.Location
		update.Message.Chat.ID = int(s.ChatId)
		// Location
		location.Latitude = s.Location.Latitude
		location.Longitude = s.Location.Latitude

		update.Message.Location = &location
		// Send report
		err := c.createReplayMsgWeather(&update)
		c.apiTelegram.Logger.Error("Can`t create report", "Error", err)
	}
}

func isUnknownCommand(update *telegrambot.Update) bool {
	command := update.Message.Text
	return infoMap[command] == "" && flagsCountryMap[command] == "" && update.Message.Location == nil
}

func (c *TelegramService) saveCommand(chatId int, command string) {
	if infoMap[command] != "" {
		c.previousCommand[chatId] = command
	}
}
