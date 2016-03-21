package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/barnybug/go-cast"
	"github.com/barnybug/go-cast/controllers"
	"github.com/barnybug/go-cast/log"
	"github.com/codegangsta/cli"
)

const UrnMedia = "urn:x-cast:com.google.cast.media"

func checkErr(err error) {
	if err != nil {
		log.Fatalln(err)
	}
}

func main() {
	commonFlags := []cli.Flag{
		cli.BoolFlag{
			Name:  "debug, d",
			Usage: "enable debug logging",
		},
		cli.StringFlag{
			Name:  "host",
			Usage: "chromecast hostname or IP (required)",
		},
		cli.IntFlag{
			Name:  "port",
			Usage: "chromecast port",
			Value: 8009,
		},
		cli.DurationFlag{
			Name:  "timeout",
			Value: 15 * time.Second,
		},
	}
	app := cli.NewApp()
	app.Name = "cast"
	app.Usage = "Command line tool for the Chromecast"
	app.Version = cast.Version
	app.Flags = commonFlags
	app.Commands = []cli.Command{
		{
			Name:      "play",
			Usage:     "play some media",
			ArgsUsage: "play url [content type]",
			Action:    cliCommand,
		},
		{
			Name:   "stop",
			Usage:  "stop playing",
			Action: cliCommand,
		},
		{
			Name:   "pause",
			Usage:  "pause playing",
			Action: cliCommand,
		},
		{
			Name:   "volume",
			Usage:  "set current volume",
			Action: cliCommand,
		},
		{
			Name:   "quit",
			Usage:  "close current app on Chromecast",
			Action: cliCommand,
		},
		{
			Name:   "script",
			Usage:  "Run the set of commands passed to stdin",
			Action: scriptCommand,
		},
		{
			Name:   "status",
			Usage:  "Get status of the Chromecast",
			Action: statusCommand,
		},
	}
	app.Run(os.Args)
	log.Println("Done")
}

func cliCommand(c *cli.Context) {
	log.Debug = c.GlobalBool("debug")
	ctx, cancel := context.WithTimeout(context.Background(), c.GlobalDuration("timeout"))
	defer cancel()
	if !checkCommand(c.Command.Name, c.Args()) {
		return
	}
	client := connect(ctx, c)
	runCommand(ctx, client, c.Command.Name, c.Args())
}

func connect(ctx context.Context, c *cli.Context) *cast.Client {
	host := c.GlobalString("host")
	log.Printf("Looking up %s...", host)
	ips, err := net.LookupIP(host)
	checkErr(err)

	client := cast.NewClient(ips[0], c.GlobalInt("port"))
	err = client.Connect(ctx)
	checkErr(err)

	log.Println("Connected")
	return client
}

func scriptCommand(c *cli.Context) {
	log.Debug = c.GlobalBool("debug")
	ctx, cancel := context.WithTimeout(context.Background(), c.GlobalDuration("timeout"))
	defer cancel()
	scanner := bufio.NewScanner(os.Stdin)
	commands := [][]string{}

	for scanner.Scan() {
		args := strings.Split(scanner.Text(), " ")
		if len(args) == 0 {
			continue
		}
		if !checkCommand(args[0], args[1:]) {
			return
		}
		commands = append(commands, args)
	}

	client := connect(ctx, c)

	for _, args := range commands {
		runCommand(ctx, client, args[0], args[1:])
	}
}

func statusCommand(c *cli.Context) {
	log.Debug = c.GlobalBool("debug")
	ctx, cancel := context.WithTimeout(context.Background(), c.GlobalDuration("timeout"))
	defer cancel()
	client := connect(ctx, c)

	status, err := client.Receiver().GetStatus(ctx)
	checkErr(err)

	if len(status.Applications) > 0 {
		for _, app := range status.Applications {
			fmt.Printf("[%s] %s\n", *app.DisplayName, *app.StatusText)
		}
	} else {
		fmt.Println("No applications running")
	}
	fmt.Printf("Volume: %.2f", *status.Volume.Level)
	if *status.Volume.Muted {
		fmt.Print("muted\n")
	} else {
		fmt.Print("\n")
	}
}

var minArgs = map[string]int{
	"play":   1,
	"pause":  0,
	"stop":   0,
	"quit":   0,
	"volume": 1,
}

var maxArgs = map[string]int{
	"play":   2,
	"pause":  0,
	"stop":   0,
	"quit":   0,
	"volume": 1,
}

func checkCommand(cmd string, args []string) bool {
	if _, ok := minArgs[cmd]; !ok {
		fmt.Printf("Command '%s' not understood\n", cmd)
		return false
	}
	if len(args) < minArgs[cmd] {
		fmt.Printf("Command '%s' requires at least %d argument(s)\n", cmd, minArgs[cmd])
		return false
	}
	if len(args) > maxArgs[cmd] {
		fmt.Printf("Command '%s' takes at most %d argument(s)\n", cmd, maxArgs[cmd])
		return false
	}
	switch cmd {
	case "play":

	case "volume":
		if err := validateFloat(args[0], 0.0, 1.0); err != nil {
			fmt.Printf("Command '%s': %s\n", cmd, err)
			return false
		}

	}
	return true
}

func validateFloat(val string, min, max float64) error {
	fval, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return fmt.Errorf("Expected a number between 0.0 and 1.0")
	}
	if fval < min {
		return fmt.Errorf("Value is below minimum: %.2f", min)
	}
	if fval > max {
		return fmt.Errorf("Value is below maximum: %.2f", max)
	}
	return nil
}

func runCommand(ctx context.Context, client *cast.Client, cmd string, args []string) {
	switch cmd {
	case "play":
		media, err := client.Media(ctx)
		checkErr(err)
		url := args[0]
		contentType := "audio/mpeg"
		if len(args) > 1 {
			contentType = args[1]
		}
		item := controllers.MediaItem{url, "BUFFERED", contentType}
		_, err = media.LoadMedia(ctx, item, 0, true, map[string]interface{}{})
		checkErr(err)

	case "pause":
		media, err := client.Media(ctx)
		checkErr(err)
		_, err = media.Pause(ctx)
		checkErr(err)

	case "stop":
		if !client.IsPlaying(ctx) {
			// if media isn't running, no media can be playing
			return
		}
		media, err := client.Media(ctx)
		checkErr(err)
		_, err = media.Stop(ctx)
		checkErr(err)

	case "volume":
		receiver := client.Receiver()
		level, _ := strconv.ParseFloat(args[0], 64)
		muted := false
		volume := controllers.Volume{Level: &level, Muted: &muted}
		_, err := receiver.SetVolume(ctx, &volume)
		checkErr(err)

	case "quit":
		receiver := client.Receiver()
		_, err := receiver.QuitApp(ctx)
		checkErr(err)

	default:
		fmt.Printf("Command '%s' not understood - ignored\n", cmd)
	}
}
