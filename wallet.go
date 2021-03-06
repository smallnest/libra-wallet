package main

import (
	"flag"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/codemaveric/libra-go/pkg/goclient"
	"github.com/codemaveric/libra-go/pkg/librawallet"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/middleware"
)

var (
	s = flag.String("s", ":8080", "server address")
)

var (
	libraClient *goclient.LibraClient
	// acc         *librawallet.Account
	// accAddr     = ""
	// mnemonic    = "present good satochi coin future media giant"
)

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

func main() {
	flag.Parse()

	initLibraClient()

	e := echo.New()
	e.Use(session.Middleware(sessions.NewCookieStore([]byte("libra"))))
	// e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(Auth())
	e.HTTPErrorHandler = errorHandler
	e.HideBanner = true

	funcMap := template.FuncMap{
		"formatNum": func(num uint64) string {
			return humanize.Comma(int64(num))
		},
		"formatLibraNum": func(num uint64) string {
			return humanize.Comma(int64(num / 1e6))
		},
	}
	t := &Template{
		templates: template.Must(template.New("main").Funcs(funcMap).ParseGlob("template/*.html")),
	}
	e.Renderer = t

	e.Static("/", "assets")

	// 查询余额
	e.GET("/", queryBalance)
	e.GET("/login", func(c echo.Context) error {
		return c.Render(http.StatusOK, "login.html", "")
	})
	e.POST("/login", func(c echo.Context) error {
		m := c.FormValue("mnemonic")
		if m != "" {
			cookie := new(http.Cookie)
			cookie.Name = "walletm"
			cookie.Value = m
			cookie.Expires = time.Now().Add(24 * time.Hour)
			c.SetCookie(cookie)
		}
		return c.Redirect(http.StatusSeeOther, "/")
	})

	e.GET("/logout", func(c echo.Context) error {
		cookie := new(http.Cookie)
		cookie.Name = "walletm"
		cookie.Value = ""
		cookie.MaxAge = -1
		c.SetCookie(cookie)
		return c.Redirect(http.StatusSeeOther, "/login")
	})

	e.GET("/events", func(c echo.Context) error {
		accAddr, _ := getAccount(c.Get("walletm").(string))
		return c.Render(http.StatusOK, "event.html", accAddr)
	})

	e.GET("/transfer", func(c echo.Context) error {
		accAddr, _ := getAccount(c.Get("walletm").(string))
		return c.Render(http.StatusOK, "transfer.html", accAddr)
	})
	e.POST("/transfer", transfer)

	e.GET("/mint", func(c echo.Context) error {
		accAddr, _ := getAccount(c.Get("walletm").(string))
		return c.Render(http.StatusOK, "mint.html", accAddr)
	})
	e.POST("/mint", mint)

	e.Logger.Fatal(e.Start(*s))
}

func errorHandler(err error, c echo.Context) {
	c.Render(http.StatusOK, "error.html", err)
}

func queryBalance(c echo.Context) error {
	accAddr, _ := getAccount(c.Get("walletm").(string))

	resp, err := libraClient.GetAccountState(accAddr)
	if err != nil {
		log.Printf("failed to get account state: %v", err)
		initLibraClient()
		return err
	}
	return c.Render(http.StatusOK, "index.html", resp)
}

func transfer(c echo.Context) error {
	transferTo := c.FormValue("transferTo")
	numberOfCoins := c.FormValue("numberOfCoins")
	amount, err := strconv.ParseUint(numberOfCoins, 10, 64)
	if err != nil {
		log.Printf("failed to parse numberOfCoins amount: %v", err)
		c.Render(http.StatusInternalServerError, "transfer_result.html", "transfer failed: "+err.Error())
		return nil
	}

	gasUnitPriceStr := c.FormValue("gas_unit_price")
	gasUnitPrice, err := strconv.ParseUint(gasUnitPriceStr, 10, 64)
	if err != nil {
		log.Printf("failed to parse gas_unit_price: %v", err)
		c.Render(http.StatusInternalServerError, "transfer_result.html", "transfer failed: "+err.Error())
		return nil
	}

	maxGasAmountStr := c.FormValue("max_gas_amount")
	maxGasAmount, err := strconv.ParseUint(maxGasAmountStr, 10, 64)
	if err != nil {
		log.Printf("failed to parse max_gas_amount: %v", err)
		c.Render(http.StatusInternalServerError, "transfer_result.html", "transfer failed: "+err.Error())
		return nil
	}

	accAddr, acc := getAccount(c.Get("walletm").(string))
	state, err := libraClient.GetAccountState(accAddr)
	if err != nil {
		log.Printf("failed to get seq: %v", err)
		c.Render(http.StatusInternalServerError, "transfer_result.html", "transfer failed: "+err.Error())
		return nil
	}
	acc.Sequence = state.SequenceNumber
	err = libraClient.TransferCoins(acc, transferTo, amount, gasUnitPrice, maxGasAmount, true)
	if err != nil {
		initLibraClient()
		log.Printf("failed to transfer: %v", err)
		c.Render(http.StatusOK, "transfer_result.html", "transfer failed: "+err.Error())
		return nil
	}

	return c.Render(http.StatusOK, "transfer_result.html", "transfer succeeded")
}

func mint(c echo.Context) error {
	accAddr, _ := getAccount(c.Get("walletm").(string))

	transferTo := c.FormValue("transferTo")
	if transferTo == "" {
		transferTo = accAddr
	}
	numberOfCoins := c.FormValue("numberOfCoins")
	amount, err := strconv.ParseUint(numberOfCoins, 10, 64)
	if err != nil {
		log.Printf("failed to parse numberOfCoins amount: %v", err)
		c.Redirect(http.StatusSeeOther, "/")
		return nil
	}

	err = libraClient.MintWithFaucetService(transferTo, amount*1e6, false)
	if err != nil {
		initLibraClient()
		log.Printf("failed to mint: %v", err)
	}

	return c.Redirect(http.StatusSeeOther, "/")
}

func getAccount(m string) (string, *librawallet.Account) {
	// account
	wallet := librawallet.NewWalletLibrary(m)
	address, childNum, err := wallet.NewAddress()
	if err != nil {
		log.Fatal(err)
	}
	accAddr := address.ToString()
	keyPair := librawallet.GenerateKeyPair(strings.Split(m, " "), childNum)
	return accAddr, librawallet.NewAccountFromKeyPair(keyPair)
}

func initLibraClient() {

	// client
	config := goclient.LibraClientConfig{
		Host:    "ac.testnet.libra.org",
		Port:    "80",
		Network: goclient.TestNet,
	}
	libraClient = goclient.NewLibraClient(config)

}
