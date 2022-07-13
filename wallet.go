package main

import (
	"fmt"
	"os"
	"io"
	"strings"
	"strconv"
	"sync"
	"time"

	d "github.com/deroholic/derogo"
	"github.com/deroproject/derohe/rpc"
	"github.com/deroproject/derohe/cryptography/crypto"
	"github.com/chzyer/readline"
)

var bridgeRegistry = "9bd7382be154ccfb981d1a0960f4c5b0227349980ec6446a2318d1401b8b738d"

var testnet bool
var wallet_password string
var wallet_file = "wallet.db"
var daemon_address = "127.0.0.1:20000"

var contracts map[string]string
var decimals map[string]int
var bridgefees map[string]uint64

var prompt_mutex sync.Mutex // prompt lock
var zerohash crypto.Hash

func parseOpt(param string) {
        s := strings.Split(param, "=")

        if s[0] == "--testnet" {
		testnet = true
        } else if len(s) > 1 && s[0] == "--daemon-address" {
                daemon_address = s[1]
        } else if len(s) > 1 && s[0] == "--wallet" {
		wallet_file = s[1]
        } else if len(s) > 1 && s[0] == "--password" {
                wallet_password = s[1]
        } else if s[0] == "--help" {
                fmt.Printf("wallet [--help] [--wallet=<wallet_file> | <private_key>] [--password=<wallet_password>] [--daemon-address=<127.0.0.1:10102>] [--testnet]\n")
                os.Exit(0)
        } else {
                fmt.Printf("invalid argument '%s', skipping\n", param)
        }
}

func walletOpts() {
        for i:= 0; i < len(os.Args[1:]); i++ {
                param := os.Args[i+1]
                if param[0] == '-' && param[1] == '-' {
                        parseOpt(param)
                } else {
                }
        }
}

func displayTokens() {
	contracts = make(map[string]string)
	decimals = make(map[string]int)
	bridgefees = make(map[string]uint64)

	vars, valid := d.DeroGetVars(bridgeRegistry)

	if valid {
		fmt.Printf("%-10s %-64s %18s\n\n", "TOKEN", "CONTRACT", "BALANCE")
		for key, value := range vars {
			s := strings.Split(key, ":")
			if s[0] == "s" {
				fee_str, _ := d.DeroGetVar(value.(string), "bridgeFee")
				fee, _ := strconv.Atoi(fee_str)

				dec_str, _ := d.DeroGetVar(value.(string), "decimals")
				dec, _ := strconv.Atoi(dec_str)

				bal := d.DeroFormatMoneyPrecision(d.DeroGetSCBal(value.(string)), dec)
				fmt.Printf("%-10s %64s %18.9f\n", s[1], value, bal)
				contracts[s[1]] = value.(string)
				decimals[s[1]] = dec
				bridgefees[s[1]] = uint64(fee)
			}
		}
		contracts["DERO"] = zerohash.String()
		decimals["DERO"] = 5
		fmt.Printf("%-10s %64s %18.9f\n", "DERO", zerohash.String(), d.DeroFormatMoneyPrecision(d.DeroGetBalance(), 5))
		fmt.Printf("\n")
	}
}

func main() {
	walletOpts()

	d.DeroInit(daemon_address)
	d.DeroWalletInit(daemon_address, false, wallet_file, wallet_password)

	displayTokens()
	commandLoop()
}

func callTransfer(scid string, dero_addr string, amount uint64) bool {
	var transfers []rpc.Transfer

	if scid == zerohash.String() {
		scid = ""
	}

	transfers = d.DeroBuildTransfers(transfers, scid, dero_addr, amount, 0)

	txid, b := d.DeroTransfer(transfers)
	if !b {
		fmt.Println("Transaction failed.")
		return false
	}

	fmt.Printf("Transaction submitted: txid = %s\n", txid)
	return true
}

func callBridge(scid string, eth_addr string, amount uint64, fee uint64) bool {
        var transfers []rpc.Transfer
        transfers = d.DeroBuildTransfers(transfers, scid, "", 0, amount)
        transfers = d.DeroBuildTransfers(transfers, zerohash.String(), "", 0, fee)

        var args rpc.Arguments
        args = append(args, rpc.Argument {"entrypoint", rpc.DataString, "Bridge"})
        args = append(args, rpc.Argument {"eth_addr", rpc.DataString, eth_addr})

	txid, b := d.DeroCallSC(scid, transfers, args)

	if !b {
		fmt.Println("Transaction failed.")
		return false
	}

	fmt.Printf("Transaction submitted: txid = %s\n", txid)
	return true
}

func transfer(words []string) {
	if len(words) != 3 {
		fmt.Println("Transfer requires 3 arguments:\n")
		printHelp()
		return
	}

	token := strings.ToUpper(words[0])
	scid := contracts[token]
	if scid == "" {
		fmt.Printf("Token '%s' not found.\n", token)
		return
	}

	amount, err := d.DeroStringToAmount(words[2], decimals[token])
	if err != nil {
		fmt.Printf("Cannot parse amount '%s'\n", words[2])
		return
	}

	a, err := d.DeroParseValidateAddress(words[1])
        if err != nil {
                fmt.Printf("Cannot parse wallet address '%s'\n", words[1])
                return
        }

	fmt.Printf("Transfer %f %s to %s\n", d.DeroFormatMoneyPrecision(amount, decimals[token]), token, words[1])

	if askContinue() {
		callTransfer(scid, a.String(), amount)
	}
}

func bridge(words []string) {
	if len(words) != 3 {
		fmt.Println("Bridge requires 3 arguments:")
		printHelp()
		return
	}

	token := strings.ToUpper(words[0])
	if token == "DERO" {
		fmt.Println("Cannot bridge DERO (yet).")
		return
	}

	scid := contracts[token]
	if scid == "" {
		fmt.Printf("Token '%s' not found.\n", token)
		return
	}

	amount, err := d.DeroStringToAmount(words[2], decimals[token])
	if err != nil {
		fmt.Printf("Cannot parse amount '%s'\n", words[2])
		return
	}

	fmt.Printf("Transfer %f %s to Ethereum address %s\n", d.DeroFormatMoneyPrecision(amount, decimals[token]), token, words[1])
	fmt.Printf("Bridge fee %f DERO\n", d.DeroFormatMoneyPrecision(bridgefees[token], 5))

	if askContinue() {
		callBridge(scid, words[1], amount, bridgefees[token])
	}
}

func printHelp() {
	fmt.Println("Available commands:")
	fmt.Println("")
	fmt.Println("help")
	fmt.Println("quit")
	fmt.Println("address")
	fmt.Println("bridge <token> <eth_address> <amount>")
	fmt.Println("transfer <token> <dero_wallet> <amount>")
	fmt.Println("balance")
}

func parseCmds(line string) bool {
	words := strings.Fields(line)

	if len(words) > 0 {
		switch strings.ToLower(words[0]) {
			case "help":
				printHelp()
			case "exit", "quit", "q":
				return true;
			case "bridge":
				bridge(words[1:])
			case "transfer":
				transfer(words[1:])
			case "balance":
				displayTokens()
			case "address":
				fmt.Printf("Wallet address %s\n", d.DeroGetAddress())
			default:
				fmt.Printf("unknown command '%s'\n", words[0])
		}
	}

	return false;
}


var completer = readline.NewPrefixCompleter(
	readline.PcItem("mode",
		readline.PcItem("vi"),
		readline.PcItem("emacs"),
	),
	readline.PcItem("balance"),
	readline.PcItem("address"),
	readline.PcItem("bye"),
	readline.PcItem("exit"),
	readline.PcItem("quit"),
	readline.PcItem("help"),
	readline.PcItem("balance"),
	readline.PcItem("transfer"),
	readline.PcItem("bridge"),
)

func filterInput(r rune) (rune, bool) {
	switch r {
	// block CtrlZ feature
	case readline.CharCtrlZ:
		return r, false
	}
	return r, true
}

var l *readline.Instance

func askContinue() bool {
	prompt_mutex.Lock()
	l.SetPrompt("Continue (N/y) ? ")
	str, err := l.Readline()
	prompt_mutex.Unlock()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	if len(str) > 0 {
		if  str[0:1] == "y" || str[0:1] == "Y" {
			return true
		}
	}

	fmt.Println("Cancelled.")
	return false
}

func commandLoop() {
	var err error

	l, err = readline.NewEx(&readline.Config{
		Prompt:          "\033[31m»\033[0m ",
		HistoryFile:     "",
		AutoComplete:    completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",

		HistorySearchFold:   true,
		FuncFilterInputRune: filterInput,
	})
	if err != nil {
		panic(err)
	}
	defer l.Close()

	go update_prompt()

	for {
		line, err := l.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			} else {
				continue
			}
		} else if err == io.EOF {
			break
		}

		line = strings.TrimSpace(line)
		words := strings.Fields(line)

		if len(words) > 0 {
			switch strings.ToLower(words[0]) {
			case "mode":
				if len(words) > 1 {
					switch words[1] + "" {
					case "vi":
						l.SetVimMode(true)
					case "emacs":
						l.SetVimMode(false)
					default:
						println("invalid mode:", line[5:])
					}
				}
			case "help", "?":
				printHelp()
			case "address":
				fmt.Printf("Wallet address %s\n", d.DeroGetAddress())
			case "bridge":
				bridge(words[1:])
			case "transfer":
				transfer(words[1:])
			case "balance":
				displayTokens()
			case "exit", "quit", "q", "bye":
				goto exit;
			case "":
			default:
				fmt.Println("unknown command: ", strconv.Quote(line))
			}
		}
	}
exit:
}

func update_prompt() {
	for {
		prompt_mutex.Lock()

		dh := d.DeroGetHeight()
		wh := d.DeroGetWalletHeight()

		p := fmt.Sprintf("%d/%d > ", wh, dh)
		l.SetPrompt(p)
		l.Refresh()

		prompt_mutex.Unlock()

		time.Sleep(100 * time.Millisecond)
	}
}
