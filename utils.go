package main

import (
	"crypto/rand"
	"math/big"
)

var numbers = []rune("0123456789")

func generateMfaToken(n int) string {
	b := make([]rune, n)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(numbers))))
		if err != nil {
			panic(err)
		}

		b[i] = numbers[num.Int64()]
	}

	return string(b)
}
