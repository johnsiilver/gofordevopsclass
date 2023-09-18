/*
Copyright © 2021 John Doak

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"io"
	"log"
	"net"
	"net/netip"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a quote from the QOTD server",
	Long: `Get requests a quote from the QOTD server. If no author is specified,
the client will get a random quote from a random author. For example:

qotd get -a "hellen keller"
or
qotd get
`,
	Run: func(cmd *cobra.Command, args []string) {
		const devAddr = "127.0.0.1:17"
		fs := cmd.Flags()

		addr := mustString(fs, "addr")
		if mustBool(fs, "dev") {
			addr = devAddr
		}

		addrNetIP, err := getAddr(addr)
		if err != nil {
			log.Fatalf("invalid remote address(%s): %s", addr, err)
		}

		author := mustString(fs, "author")
		if len(author) == 0 {
			author = "none"
		}

		conn, err := net.Dial("tcp", addrNetIP.String())
		if err != nil {
			log.Fatal(err)
		}

		handleAuthor(conn, author)
	},
}

func init() {
	rootCmd.AddCommand(getCmd)

	getCmd.Flags().StringP("author", "a", "",
		"Specify the author to get a quote for")

	rootCmd.Flags().StringP("username", "u", "", "Username (required if password is set)")
	rootCmd.Flags().StringP("password", "p", "", "Password (required if username is set)")
	rootCmd.MarkFlagsRequiredTogether("username", "password")

	rootCmd.Flags().Bool("json", false, "Output in JSON")
	rootCmd.Flags().Bool("yaml", false, "Output in YAML")
	rootCmd.MarkFlagsMutuallyExclusive("json", "yaml")
}

func mustString(fs *pflag.FlagSet, name string) string {
	v, err := fs.GetString(name)
	if err != nil {
		panic(err)
	}
	return v
}

func mustBool(fs *pflag.FlagSet, name string) bool {
	v, err := fs.GetBool(name)
	if err != nil {
		panic(err)
	}
	return v
}

func getAddr(s string) (netip.AddrPort, error) {
	if strings.Contains(s, ":") {
		return netip.ParseAddrPort(s)
	}
	return netip.ParseAddrPort(s + ":17")
}

func handleAuthor(conn net.Conn, author string) {
	defer conn.Close()

	if author != "none" {
		// Write the author to the connection.
		_, err := io.WriteString(conn, author+"\n")
		if err != nil {
			log.Fatal(err)
		}
	}

	// Read from the connection and write to stdout.
	b := make([]byte, 1024)
	n, err := io.ReadFull(conn, b)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		log.Fatal(err)
	}
	color.Blue(string(b[:n]))
}
