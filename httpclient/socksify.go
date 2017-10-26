package httpclient

import (
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"strings"

	proxy "github.com/cloudfoundry/socks5-proxy"

	goproxy "golang.org/x/net/proxy"
)

type DialFunc func(network, address string) (net.Conn, error)

func (f DialFunc) Dial(network, address string) (net.Conn, error) { return f(network, address) }

func SOCKS5DialFuncFromEnvironment(origDialer DialFunc) DialFunc {
	allProxy := os.Getenv("BOSH_ALL_PROXY")
	if len(allProxy) == 0 {
		return origDialer
	}

	if strings.HasPrefix(allProxy, "ssh+") && strings.Contains(allProxy, "?private-key=") {
		allProxy = strings.TrimPrefix(allProxy, "ssh+socks5://")

		proxyBaseURL := strings.Split(allProxy, "?")[0]
		proxySSHKeyPath := strings.Split(allProxy, "=")[1]

		proxySSHKey, err := ioutil.ReadFile(proxySSHKeyPath)
		if err != nil {
			return origDialer
		}

		socks5Proxy := proxy.NewSocks5Proxy(proxy.NewHostKeyGetter())
		dialer, err := socks5Proxy.Dialer(string(proxySSHKey), proxyBaseURL)
		if err != nil {
			return origDialer
		}

		return func(network, address string) (net.Conn, error) {
			return dialer(network, address)
		}
	}

	proxyURL, err := url.Parse(allProxy)
	if err != nil {
		return origDialer
	}

	proxy, err := goproxy.FromURL(proxyURL, origDialer)
	if err != nil {
		return origDialer
	}

	noProxy := os.Getenv("no_proxy")
	if len(noProxy) == 0 {
		return proxy.Dial
	}

	perHost := goproxy.NewPerHost(proxy, origDialer)
	perHost.AddFromString(noProxy)

	return perHost.Dial
}
