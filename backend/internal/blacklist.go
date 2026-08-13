package internal

import (
	"bufio"
	"log"
	"os"
	"strings"
)

var BlackListedIPs = make(map[string]struct{})

// LoadBlacklist Load blacklist from the txt file
func LoadBlacklist(filePath string) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Failed to open blacklist file: %v\n", err)
		return
	}
	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Printf("Failed to close file: %v\n", err)
			return
		}
	}(file)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		ip := strings.TrimSpace(scanner.Text())
		if ip != "" {
			BlackListedIPs[ip] = struct{}{}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Failed to read blacklist file: %v\n", err)
		return
	}
}

// IsBlacklisted Check if IP is blacklisted or not
func IsBlacklisted(ip string) bool {
	_, exists := BlackListedIPs[ip]
	return exists
}
