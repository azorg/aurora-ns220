// core-daemon — обёртка бизнес-логики для не-Go фронтендов (Qt/QML, CLI).
//
// Режимы:
//   core-daemon --stdio        JSON-lines через stdin/stdout (для QProcess)
//   core-daemon --socket PATH  JSON-lines через unix-сокет (несколько клиентов)
//   core-daemon --hello [имя]  одноразовый вызов (для отладки)
package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"aurora/hybrid/core"
)

func main() {
	stdio := flag.Bool("stdio", false, "JSON-lines через stdin/stdout")
	sock := flag.String("socket", "", "JSON-lines через unix-сокет")
	hello := flag.Bool("hello", false, "одноразовый вызов Hello")
	flag.Parse()

	svc := core.New()

	if *hello {
		name := ""
		if flag.NArg() > 0 {
			name = flag.Arg(0)
		}
		fmt.Println(svc.Hello(name))
		return
	}

	switch {
	case *stdio:
		if err := svc.Serve(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "serve:", err)
			os.Exit(1)
		}
	case *sock != "":
		_ = os.Remove(*sock)
		ln, err := net.Listen("unix", *sock)
		if err != nil {
			fmt.Fprintln(os.Stderr, "listen:", err)
			os.Exit(1)
		}
		fmt.Println("listening on", *sock)
		for {
			conn, err := ln.Accept()
			if err != nil {
				continue
			}
			go func(c net.Conn) {
				defer c.Close()
				_ = svc.Serve(c, c)
			}(conn)
		}
	default:
		fmt.Fprintln(os.Stderr, "укажите --stdio | --socket PATH | --hello")
		os.Exit(2)
	}
}
