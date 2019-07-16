package cmd_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/armon/go-socks5"
	"github.com/stefanoj3/dirstalk/pkg/common/test"
	"github.com/stretchr/testify/assert"
)

const socks5TestServerHost = "127.0.0.1:8899"

func TestScanCommand(t *testing.T) {
	logger, _ := test.NewLogger()

	c, err := createCommand(logger)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	testServer, serverAssertion := test.NewServerWithAssertion(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/test/" {
				w.WriteHeader(http.StatusOK)
				return
			}

			w.WriteHeader(http.StatusNotFound)

		}),
	)
	defer testServer.Close()

	_, _, err = executeCommand(
		c,
		"scan",
		testServer.URL,
		"--dictionary",
		"testdata/dict2.txt",
		"-v",
		"--http-timeout",
		"300",
	)
	assert.NoError(t, err)

	assert.Equal(t, 8, serverAssertion.Len())

	requestsMap := map[string]string{}

	serverAssertion.Range(func(_ int, r http.Request) {
		requestsMap[r.URL.Path] = r.Method
	})

	expectedRequests := map[string]string{
		"/test/":               http.MethodGet,
		"/test/home":           http.MethodGet,
		"/test/blabla":         http.MethodGet,
		"/test/home/index.php": http.MethodGet,
		"/test/test/":          http.MethodGet,

		"/home":           http.MethodGet,
		"/blabla":         http.MethodGet,
		"/home/index.php": http.MethodGet,
	}

	assert.Equal(t, expectedRequests, requestsMap)
}

func TestScanWithRemoteDictionary(t *testing.T) {
	logger, _ := test.NewLogger()

	c, err := createCommand(logger)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	dictionaryServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dict := `home
home/index.php
blabla
`
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(dict))
		}),
	)
	defer dictionaryServer.Close()

	testServer, serverAssertion := test.NewServerWithAssertion(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	)
	defer testServer.Close()

	_, _, err = executeCommand(
		c,
		"scan",
		testServer.URL,
		"--dictionary",
		dictionaryServer.URL,
		"--http-timeout",
		"300",
	)
	assert.NoError(t, err)

	assert.Equal(t, 3, serverAssertion.Len())
}

func TestScanWithUserAgentFlag(t *testing.T) {
	const testUserAgent = "my_test_user_agent"

	logger, loggerBuffer := test.NewLogger()

	c, err := createCommand(logger)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	testServer, serverAssertion := test.NewServerWithAssertion(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	)
	defer testServer.Close()

	_, _, err = executeCommand(
		c,
		"scan",
		testServer.URL,
		"--user-agent",
		testUserAgent,
		"--dictionary",
		"testdata/dict.txt",
		"--http-timeout",
		"300",
	)
	assert.NoError(t, err)

	assert.Equal(t, 3, serverAssertion.Len())
	serverAssertion.Range(func(_ int, r http.Request) {
		assert.Equal(t, testUserAgent, r.Header.Get("User-Agent"))
	})

	// to ensure we print the user agent to the cli
	assert.Contains(t, loggerBuffer.String(), testUserAgent)
}

func TestScanWithCookies(t *testing.T) {
	logger, loggerBuffer := test.NewLogger()

	c, err := createCommand(logger)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	testServer, serverAssertion := test.NewServerWithAssertion(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	)
	defer testServer.Close()

	_, _, err = executeCommand(
		c,
		"scan",
		testServer.URL,
		"--cookie",
		"name1=val1",
		"--cookie",
		"name2=val2",
		"--dictionary",
		"testdata/dict.txt",
		"--http-timeout",
		"300",
	)
	assert.NoError(t, err)

	serverAssertion.Range(func(_ int, r http.Request) {
		assert.Equal(t, 2, len(r.Cookies()))

		assert.Equal(t, r.Cookies()[0].Name, "name1")
		assert.Equal(t, r.Cookies()[0].Value, "val1")

		assert.Equal(t, r.Cookies()[1].Name, "name2")
		assert.Equal(t, r.Cookies()[1].Value, "val2")
	})

	// to ensure we print the cookies to the cli
	assert.Contains(t, loggerBuffer.String(), "name1=val1")
	assert.Contains(t, loggerBuffer.String(), "name2=val2")
}

func TestWhenProvidingCookiesInWrongFormatShouldErr(t *testing.T) {
	const malformedCookie = "gibberish"

	logger, _ := test.NewLogger()

	c, err := createCommand(logger)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	testServer, serverAssertion := test.NewServerWithAssertion(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	)
	defer testServer.Close()

	_, _, err = executeCommand(
		c,
		"scan",
		testServer.URL,
		"--cookie",
		malformedCookie,
		"--dictionary",
		"testdata/dict.txt",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cookie format is invalid")
	assert.Contains(t, err.Error(), malformedCookie)

	assert.Equal(t, 0, serverAssertion.Len())
}

func TestScanWithCookieJar(t *testing.T) {
	const (
		serverCookieName  = "server_cookie_name"
		serverCookieValue = "server_cookie_value"
	)

	logger, _ := test.NewLogger()

	c, err := createCommand(logger)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	once := sync.Once{}
	testServer, serverAssertion := test.NewServerWithAssertion(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			once.Do(func() {
				http.SetCookie(
					w,
					&http.Cookie{
						Name:    serverCookieName,
						Value:   serverCookieValue,
						Expires: time.Now().AddDate(0, 1, 0),
					},
				)
			})
		}),
	)
	defer testServer.Close()

	_, _, err = executeCommand(
		c,
		"scan",
		testServer.URL,
		"--use-cookie-jar",
		"--dictionary",
		"testdata/dict.txt",
		"--http-timeout",
		"300",
		"-t",
		"1",
	)
	assert.NoError(t, err)

	serverAssertion.Range(func(index int, r http.Request) {
		if index == 0 { // first request should have no cookies
			assert.Equal(t, 0, len(r.Cookies()))
			return
		}

		assert.Equal(t, 1, len(r.Cookies()))
		assert.Equal(t, r.Cookies()[0].Name, serverCookieName)
		assert.Equal(t, r.Cookies()[0].Value, serverCookieValue)
	})
}

func TestScanWithUnknownFlagShouldErr(t *testing.T) {
	logger, _ := test.NewLogger()

	c, err := createCommand(logger)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	testServer, serverAssertion := test.NewServerWithAssertion(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	)
	defer testServer.Close()

	_, _, err = executeCommand(
		c,
		"scan",
		testServer.URL,
		"--gibberishflag",
		"--dictionary",
		"testdata/dict.txt",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flag")

	assert.Equal(t, 0, serverAssertion.Len())
}

func TestScanWithHeaders(t *testing.T) {
	logger, loggerBuffer := test.NewLogger()

	c, err := createCommand(logger)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	testServer, serverAssertion := test.NewServerWithAssertion(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	)
	defer testServer.Close()

	_, _, err = executeCommand(
		c,
		"scan",
		testServer.URL,
		"--header",
		"Accept-Language: en-US,en;q=0.5",
		"--header",
		`"Authorization: Bearer 123"`,
		"--dictionary",
		"testdata/dict.txt",
		"--http-timeout",
		"300",
	)
	assert.NoError(t, err)

	serverAssertion.Range(func(_ int, r http.Request) {
		assert.Equal(t, 2, len(r.Header))

		assert.Equal(t, "en-US,en;q=0.5", r.Header.Get("Accept-Language"))
		assert.Equal(t, "Bearer 123", r.Header.Get("Authorization"))
	})

	// to ensure we print the headers to the cli
	assert.Contains(t, loggerBuffer.String(), "Accept-Language")
	assert.Contains(t, loggerBuffer.String(), "Authorization")
	assert.Contains(t, loggerBuffer.String(), "Bearer 123")
}

func TestScanWithMalformedHeaderShouldErr(t *testing.T) {
	const malformedHeader = "gibberish"

	logger, _ := test.NewLogger()

	c, err := createCommand(logger)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	testServer, serverAssertion := test.NewServerWithAssertion(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	)
	defer testServer.Close()

	_, _, err = executeCommand(
		c,
		"scan",
		testServer.URL,
		"--header",
		"Accept-Language: en-US,en;q=0.5",
		"--header",
		malformedHeader,
		"--dictionary",
		"testdata/dict.txt",
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), malformedHeader)
	assert.Contains(t, err.Error(), "header is in invalid format")

	assert.Equal(t, 0, serverAssertion.Len())
}

func TestStartScanWithSocks5ShouldFindResultsWhenAServerIsAvailable(t *testing.T) {
	logger, _ := test.NewLogger()

	c, err := createCommand(logger)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	testServer, serverAssertion := test.NewServerWithAssertion(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	)
	defer testServer.Close()

	socks5Server := startSocks5TestServer(t)
	defer socks5Server.Close()

	_, _, err = executeCommand(
		c,
		"scan",
		testServer.URL,
		"--dictionary",
		"testdata/dict.txt",
		"-v",
		"--http-timeout",
		"300",
		"--socks5",
		socks5TestServerHost,
	)
	assert.NoError(t, err)

	assert.Equal(t, 3, serverAssertion.Len())
}

func TestShouldFailToScanWithAnUnreachableSocks5Server(t *testing.T) {
	logger, loggerBuffer := test.NewLogger()

	c, err := createCommand(logger)
	assert.NoError(t, err)
	assert.NotNil(t, c)

	testServer, serverAssertion := test.NewServerWithAssertion(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	)
	defer testServer.Close()

	socks5Server := startSocks5TestServer(t)
	defer socks5Server.Close()

	_, _, err = executeCommand(
		c,
		"scan",
		testServer.URL,
		"--dictionary",
		"testdata/dict.txt",
		"-v",
		"--http-timeout",
		"300",
		"--socks5",
		"127.0.0.1:9555", // invalid
	)
	assert.NoError(t, err)

	assert.Equal(t, 0, serverAssertion.Len())
	assert.Contains(t, loggerBuffer.String(), "failed to perform request")
	assert.Contains(t, loggerBuffer.String(), "socks connect tcp")
	assert.Contains(t, loggerBuffer.String(), "connect: connection refused")
}

func startSocks5TestServer(t *testing.T) net.Listener {
	conf := &socks5.Config{}
	server, err := socks5.New(conf)
	if err != nil {
		t.Fatalf("failed to create socks5: %s", err.Error())
	}

	listener, err := net.Listen("tcp", socks5TestServerHost)
	if err != nil {
		t.Fatalf("failed to create listener: %s", err.Error())
	}

	go func() {
		// Create SOCKS5 proxy on localhost port 8000
		if err := server.Serve(listener); err != nil {
			t.Logf("socks5 stopped serving: %s", err.Error())
		}
	}()

	return listener
}