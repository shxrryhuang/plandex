package validation

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// DatabaseConfig holds database configuration from environment
type DatabaseConfig struct {
	DatabaseURL string
	Host        string
	Port        string
	User        string
	Password    string
	Name        string
}

// LoadDatabaseConfig loads database configuration from environment variables
func LoadDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Host:        os.Getenv("DB_HOST"),
		Port:        os.Getenv("DB_PORT"),
		User:        os.Getenv("DB_USER"),
		Password:    os.Getenv("DB_PASSWORD"),
		Name:        os.Getenv("DB_NAME"),
	}
}

// ValidateDatabase performs comprehensive database validation
func ValidateDatabase(ctx context.Context) *ValidationResult {
	result := &ValidationResult{}

	config := LoadDatabaseConfig()

	// Check if individual vars are partially set (common mistake) - check this FIRST
	if !config.HasDatabaseURL() && config.HasSomeIndividualVars() && !config.HasIndividualVars() {
		missing := config.GetMissingIndividualVars()
		result.AddError(&ValidationError{
			Category:    CategoryDatabase,
			Severity:    SeverityCritical,
			Summary:     "Incomplete database configuration",
			Details:     fmt.Sprintf("Some DB_* variables are set but these are missing: %s", strings.Join(missing, ", ")),
			Impact:      "Plandex server cannot start with incomplete database configuration.",
			Solution:    "Set all required DB_* variables or use DATABASE_URL instead.",
			RelatedVars: missing,
			Example: `export DB_HOST="localhost"
export DB_PORT="5432"
export DB_USER="plandex_user"
export DB_PASSWORD="secure_password"
export DB_NAME="plandex"`,
		})
		return result
	}

	// Check if database configuration is provided
	if !config.HasDatabaseURL() && !config.HasIndividualVars() {
		result.AddError(&ValidationError{
			Category: CategoryDatabase,
			Severity: SeverityCritical,
			Summary:  "No database configuration found",
			Details:  "Neither DATABASE_URL nor individual DB_* environment variables are set.",
			Impact:   "Plandex server cannot start without a database connection.",
			Solution: "Set either DATABASE_URL or all of DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, and DB_NAME.",
			Example: `Option 1: Using DATABASE_URL
  export DATABASE_URL="postgres://user:password@localhost:5432/plandex"

Option 2: Using individual variables
  export DB_HOST="localhost"
  export DB_PORT="5432"
  export DB_USER="plandex_user"
  export DB_PASSWORD="secure_password"
  export DB_NAME="plandex"`,
			RelatedVars: []string{"DATABASE_URL", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME"},
		})
		return result
	}

	// Get the connection string
	dbURL, err := config.GetConnectionString()
	if err != nil {
		result.AddError(NewDatabaseError(
			"Invalid database URL format",
			err.Error(),
			"Fix the DATABASE_URL format or use individual DB_* variables.",
			err,
		))
		return result
	}

	// Validate connection string format
	if validateErr := validateConnectionStringFormat(dbURL); validateErr != nil {
		result.AddError(validateErr)
		return result
	}

	// Test database connectivity
	if connectErr := testDatabaseConnection(ctx, dbURL); connectErr != nil {
		result.AddError(connectErr)
		return result
	}

	return result
}

// HasDatabaseURL returns true if DATABASE_URL is set
func (c *DatabaseConfig) HasDatabaseURL() bool {
	return c.DatabaseURL != ""
}

// HasIndividualVars returns true if all individual DB_* vars are set
func (c *DatabaseConfig) HasIndividualVars() bool {
	return c.Host != "" && c.Port != "" && c.User != "" && c.Password != "" && c.Name != ""
}

// HasSomeIndividualVars returns true if at least one individual DB_* var is set
func (c *DatabaseConfig) HasSomeIndividualVars() bool {
	return c.Host != "" || c.Port != "" || c.User != "" || c.Password != "" || c.Name != ""
}

// GetMissingIndividualVars returns a list of missing individual DB_* vars
func (c *DatabaseConfig) GetMissingIndividualVars() []string {
	var missing []string
	if c.Host == "" {
		missing = append(missing, "DB_HOST")
	}
	if c.Port == "" {
		missing = append(missing, "DB_PORT")
	}
	if c.User == "" {
		missing = append(missing, "DB_USER")
	}
	if c.Password == "" {
		missing = append(missing, "DB_PASSWORD")
	}
	if c.Name == "" {
		missing = append(missing, "DB_NAME")
	}
	return missing
}

// GetConnectionString returns the database connection string
func (c *DatabaseConfig) GetConnectionString() (string, error) {
	if c.DatabaseURL != "" {
		return c.DatabaseURL, nil
	}

	if c.HasIndividualVars() {
		encodedPassword := url.QueryEscape(c.Password)
		return fmt.Sprintf("postgres://%s:%s@%s:%s/%s", c.User, encodedPassword, c.Host, c.Port, c.Name), nil
	}

	return "", fmt.Errorf("no database configuration available")
}

// validateConnectionStringFormat validates the connection string format
func validateConnectionStringFormat(dbURL string) *ValidationError {
	// Parse the URL
	u, err := url.Parse(dbURL)
	if err != nil {
		return &ValidationError{
			Category: CategoryDatabase,
			Severity: SeverityCritical,
			Summary:  "Invalid DATABASE_URL format",
			Details:  fmt.Sprintf("Failed to parse DATABASE_URL: %v", err),
			Impact:   "Plandex server cannot connect to database with invalid URL format.",
			Solution: "Ensure DATABASE_URL follows the format: postgres://user:password@host:port/database",
			Example: `export DATABASE_URL="postgres://plandex_user:secure_password@localhost:5432/plandex"

For special characters in password, URL-encode them:
  # = %23  @ = %40  : = %3A  / = %2F  ? = %3F`,
			RelatedVars: []string{"DATABASE_URL"},
			Err:         err,
		}
	}

	// Check scheme
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return &ValidationError{
			Category:    CategoryDatabase,
			Severity:    SeverityCritical,
			Summary:     "Invalid database URL scheme",
			Details:     fmt.Sprintf("URL scheme is '%s', expected 'postgres' or 'postgresql'", u.Scheme),
			Impact:      "Plandex only supports PostgreSQL databases.",
			Solution:    "Use a PostgreSQL connection string starting with postgres:// or postgresql://",
			Example:     `export DATABASE_URL="postgres://user:password@localhost:5432/plandex"`,
			RelatedVars: []string{"DATABASE_URL"},
		}
	}

	// Check if host is present
	if u.Host == "" {
		return &ValidationError{
			Category:    CategoryDatabase,
			Severity:    SeverityCritical,
			Summary:     "Missing database host in URL",
			Details:     "DATABASE_URL does not contain a host",
			Impact:      "Cannot connect to database without a host.",
			Solution:    "Include the database host and port in the URL",
			Example:     `export DATABASE_URL="postgres://user:password@localhost:5432/plandex"`,
			RelatedVars: []string{"DATABASE_URL"},
		}
	}

	// Check if database name is present
	if u.Path == "" || u.Path == "/" {
		return &ValidationError{
			Category:    CategoryDatabase,
			Severity:    SeverityCritical,
			Summary:     "Missing database name in URL",
			Details:     "DATABASE_URL does not specify a database name",
			Impact:      "Cannot connect without knowing which database to use.",
			Solution:    "Add the database name to the end of the URL",
			Example:     `export DATABASE_URL="postgres://user:password@localhost:5432/plandex"`,
			RelatedVars: []string{"DATABASE_URL"},
		}
	}

	return nil
}

// testDatabaseConnection attempts to connect to the database
func testDatabaseConnection(ctx context.Context, dbURL string) *ValidationError {
	// Add timeout to context if not already set
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Try to open connection
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return &ValidationError{
			Category: CategoryDatabase,
			Severity: SeverityCritical,
			Summary:  "Failed to initialize database connection",
			Details:  fmt.Sprintf("Error: %v", err),
			Impact:   "Plandex server cannot start without a valid database connection.",
			Solution: "Check that the database URL is correct and the database server is accessible.",
			Example: `Common issues:
  1. Wrong credentials - verify username and password
  2. Database doesn't exist - create it first:
     createdb plandex
  3. PostgreSQL not running - start the database:
     sudo systemctl start postgresql
  4. Wrong host/port - verify the database is listening on the specified address`,
			RelatedVars: []string{"DATABASE_URL"},
			Err:         err,
		}
	}
	defer db.Close()

	// Test the connection with ping
	if err := db.PingContext(ctx); err != nil {
		// Try to provide more specific error messages
		errMsg := err.Error()
		var solution string
		var details string

		switch {
		case strings.Contains(errMsg, "connection refused"):
			details = "Database server is not accepting connections."
			solution = `The database server may not be running or is not accessible:
  1. Check if PostgreSQL is running:
     systemctl status postgresql  # Linux
     brew services list            # macOS
  2. Verify the host and port are correct
  3. Check firewall settings if connecting to a remote host`

		case strings.Contains(errMsg, "authentication failed") || strings.Contains(errMsg, "password authentication failed"):
			details = "Database credentials are invalid."
			solution = `Fix the database authentication:
  1. Verify username and password are correct
  2. Check PostgreSQL user exists:
     psql -U postgres -c "\du"
  3. Update pg_hba.conf if needed to allow authentication method`

		case strings.Contains(errMsg, "does not exist"):
			details = "The specified database does not exist."
			solution = `Create the database:
  1. Using psql:
     psql -U postgres -c "CREATE DATABASE plandex;"
  2. Or using createdb:
     createdb -U postgres plandex
  3. Then restart Plandex`

		case strings.Contains(errMsg, "timeout"):
			details = "Connection to database timed out."
			solution = `Check network connectivity:
  1. Verify the database host is reachable:
     ping <database-host>
  2. Check if port is open:
     telnet <database-host> 5432
  3. Review firewall rules
  4. For cloud databases, check security groups/firewall rules`

		case strings.Contains(errMsg, "too many clients"):
			details = "Database has reached maximum connections."
			solution = `Reduce connection count or increase max_connections:
  1. Check current connections:
     psql -U postgres -c "SELECT count(*) FROM pg_stat_activity;"
  2. Increase max_connections in postgresql.conf
  3. Kill idle connections if needed`

		default:
			details = fmt.Sprintf("Failed to connect to database: %v", err)
			solution = `Troubleshoot the connection:
  1. Verify PostgreSQL is running
  2. Check connection credentials
  3. Ensure database exists
  4. Review PostgreSQL logs for more details`
		}

		return &ValidationError{
			Category:    CategoryDatabase,
			Severity:    SeverityCritical,
			Summary:     "Cannot connect to database",
			Details:     details,
			Impact:      "Plandex server cannot start without a working database connection.",
			Solution:    solution,
			RelatedVars: []string{"DATABASE_URL"},
			Err:         err,
		}
	}

	// Test a simple query to ensure we can execute commands
	var result int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&result); err != nil {
		return &ValidationError{
			Category:    CategoryDatabase,
			Severity:    SeverityCritical,
			Summary:     "Cannot execute queries on database",
			Details:     fmt.Sprintf("Connected to database but failed to execute test query: %v", err),
			Impact:      "Plandex requires ability to execute queries on the database.",
			Solution:    "Ensure the database user has appropriate permissions to execute queries.",
			RelatedVars: []string{"DATABASE_URL"},
			Err:         err,
		}
	}

	return nil
}
