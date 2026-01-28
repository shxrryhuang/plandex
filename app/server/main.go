package main

import (
	"log"
)

// main is the entry point that delegates to the safe validation pipeline
// This preserves backward compatibility while enabling optional validation
func main() {
	// Configure the default logger to include milliseconds in timestamps
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	// Use the safe pipeline that respects feature flags
	// When validation is disabled: original behavior
	// When validation is enabled: new validation system
	mainSafe()
}
