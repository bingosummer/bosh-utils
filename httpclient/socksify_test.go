package httpclient_test

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sync"

	"errors"

	. "github.com/cloudfoundry/bosh-utils/httpclient"
	proxy "github.com/cloudfoundry/socks5-proxy"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Socksify", func() {
	var (
		proxyDialer *FakeProxyDialer
		origDial    DialFunc
		dialFunc    DialFunc
	)

	BeforeEach(func() {
		proxyDialer = &FakeProxyDialer{}
		origDial = DialFunc(func(x, y string) (net.Conn, error) {
			return nil, errors.New("original dialer")
		})
		dialFunc = SOCKS5DialFuncFromEnvironment(origDial, proxyDialer)
	})
	Context("When BOSH_ALL_PROXY is not set", func() {
		It("Returns the dialer that was passed in", func() {
			_, err := dialFunc("", "")
			Expect(err).To(MatchError("original dialer"))
			Expect(proxyDialer.DialerCall.CallCount).To(Equal(0))
		})
	})

	Context("When BOSH_ALL_PROXY is set", func() {
		Context("When BOSH_ALL_PROXY is prefixed with ssh+", func() {
			BeforeEach(func() {
				proxyDialer.DialerCall.Returns.DialFunc = proxy.DialFunc(func(x, y string) (net.Conn, error) {
					return nil, errors.New("proxy dialer")
				})
				tempDir, err := ioutil.TempDir("", "")
				Expect(err).NotTo(HaveOccurred())
				privateKeyPath := filepath.Join(tempDir, "test.key")
				err = ioutil.WriteFile(privateKeyPath, []byte("some-key"), 0600)
				Expect(err).NotTo(HaveOccurred())
				os.Setenv("BOSH_ALL_PROXY", fmt.Sprintf("ssh+socks5://localhost:12345?private-key=%s", privateKeyPath))

				dialFunc = SOCKS5DialFuncFromEnvironment(origDial, proxyDialer)
			})

			Context("When no username is given in the URL", func() {
				It("Returns a function that creates a socks5 proxy dialer for user ''", func() {
					_, err := dialFunc("", "")
					Expect(err).To(MatchError("proxy dialer"))
					Expect(proxyDialer.DialerCall.CallCount).To(Equal(1))
					Expect(proxyDialer.DialerCall.Receives.Username).To(Equal(""))
					Expect(proxyDialer.DialerCall.Receives.Key).To(Equal("some-key"))
					Expect(proxyDialer.DialerCall.Receives.URL).To(Equal("localhost:12345"))
				})
			})

			Context("When a 'custom-username' is given in the URL", func() {
				JustBeforeEach(func() {
					tempDir, err := ioutil.TempDir("", "")
					Expect(err).NotTo(HaveOccurred())
					privateKeyPath := filepath.Join(tempDir, "test.key")
					err = ioutil.WriteFile(privateKeyPath, []byte("some-key"), 0600)
					Expect(err).NotTo(HaveOccurred())
					os.Setenv("BOSH_ALL_PROXY", fmt.Sprintf("ssh+socks5://custom-username@localhost:12345?private-key=%s", privateKeyPath))
					dialFunc = SOCKS5DialFuncFromEnvironment(origDial, proxyDialer)
				})
				It("Returns a function that creates a socks5 proxy dialer for user 'custom-username'", func() {
					_, err := dialFunc("", "")
					Expect(err).To(MatchError("proxy dialer"))
					Expect(proxyDialer.DialerCall.CallCount).To(Equal(1))
					Expect(proxyDialer.DialerCall.Receives.Username).To(Equal("custom-username"))
					Expect(proxyDialer.DialerCall.Receives.Key).To(Equal("some-key"))
					Expect(proxyDialer.DialerCall.Receives.URL).To(Equal("localhost:12345"))
				})
			})

			It("Can be called multiple times and only create the dialer once", func() {
				_, err := dialFunc("", "")
				Expect(err).To(MatchError("proxy dialer"))
				_, err = dialFunc("", "")
				Expect(err).To(MatchError("proxy dialer"))
				Expect(proxyDialer.DialerCall.CallCount).To(Equal(1))
				Expect(proxyDialer.DialerCall.Receives.Username).To(Equal(""))
				Expect(proxyDialer.DialerCall.Receives.Key).To(Equal("some-key"))
				Expect(proxyDialer.DialerCall.Receives.URL).To(Equal("localhost:12345"))
			})

			It("Can be concurrently (run ginkgo with -race flag)", func() {
				errs := make(chan error)
				for i := 0; i < 20; i++ {
					go func() {
						_, err := dialFunc("", "")
						errs <- err
					}()
				}
				for i := 0; i < 20; i++ {
					err := <-errs
					Expect(err).To(MatchError("proxy dialer"))
				}
				Expect(proxyDialer.DialerCall.CallCount).To(Equal(1))
				Expect(proxyDialer.DialerCall.Receives.Username).To(Equal(""))
				Expect(proxyDialer.DialerCall.Receives.Key).To(Equal("some-key"))
				Expect(proxyDialer.DialerCall.Receives.URL).To(Equal("localhost:12345"))
			})

			Context("when the URL after the ssh+ prefix cannot be parsed", func() {
				BeforeEach(func() {
					os.Setenv("BOSH_ALL_PROXY", fmt.Sprintf("ssh+:cannot-start-with-colon"))
					dialFunc = SOCKS5DialFuncFromEnvironment(origDial, proxyDialer)
				})
				It("returns the dialer that was passed in", func() {
					_, err := dialFunc("", "")
					Expect(err).To(MatchError("original dialer"))
				})
			})

			Context("when the query params in the URL cannot be parsed", func() {
				BeforeEach(func() {
					os.Setenv("BOSH_ALL_PROXY", fmt.Sprintf("ssh+socks5://localhost:12345?foo=%%"))
					dialFunc = SOCKS5DialFuncFromEnvironment(origDial, proxyDialer)
				})
				It("returns the dialer that was passed in", func() {
					_, err := dialFunc("", "")
					Expect(err).To(MatchError("original dialer"))
				})
			})

			Context("when the query params do not contain the private key path", func() {
				BeforeEach(func() {
					os.Setenv("BOSH_ALL_PROXY", fmt.Sprintf("ssh+socks5://localhost:12345?foo=bar"))
					dialFunc = SOCKS5DialFuncFromEnvironment(origDial, proxyDialer)
				})
				It("returns the dialer that was passed in", func() {
					_, err := dialFunc("", "")
					Expect(err).To(MatchError("original dialer"))
				})
			})

			Context("when no key exists at the private key path", func() {
				BeforeEach(func() {
					os.Setenv("BOSH_ALL_PROXY", fmt.Sprintf("ssh+socks5://localhost:12345?private-key=/no/file/here"))
					dialFunc = SOCKS5DialFuncFromEnvironment(origDial, proxyDialer)
				})
				It("returns the dialer that was passed in", func() {
					_, err := dialFunc("", "")
					Expect(err).To(MatchError("original dialer"))
				})
			})
		})

		Context("When BOSH_ALL_PROXY is *not* prefixed with ssh+", func() {
			// Happy paths not tested
			Context("when the URL cannot be parsed", func() {
				BeforeEach(func() {
					os.Setenv("BOSH_ALL_PROXY", fmt.Sprintf(":cannot-start-with-colon"))
					dialFunc = SOCKS5DialFuncFromEnvironment(origDial, proxyDialer)
				})
				It("returns the dialer that was passed in", func() {
					_, err := dialFunc("", "")
					Expect(err).To(MatchError("original dialer"))
				})
			})

			Context("when the URL is not a valid proxy scheme", func() {
				BeforeEach(func() {
					os.Setenv("BOSH_ALL_PROXY", fmt.Sprintf("foo://cannot-start-with-colon"))
					dialFunc = SOCKS5DialFuncFromEnvironment(origDial, proxyDialer)
				})
				It("returns the dialer that was passed in", func() {
					_, err := dialFunc("", "")
					Expect(err).To(MatchError("original dialer"))
				})
			})
		})
	})
})

type FakeProxyDialer struct {
	DialerCall struct {
		CallCount int
		Receives  struct {
			Username string
			Key      string
			URL      string
		}
		Returns struct {
			DialFunc proxy.DialFunc
			Error    error
		}
	}
	mut sync.Mutex
}

func (p *FakeProxyDialer) Dialer(username, key, url string) (proxy.DialFunc, error) {
	p.mut.Lock()
	defer p.mut.Unlock()

	p.DialerCall.CallCount++
	p.DialerCall.Receives.Username = username
	p.DialerCall.Receives.Key = key
	p.DialerCall.Receives.URL = url

	return p.DialerCall.Returns.DialFunc, p.DialerCall.Returns.Error
}
