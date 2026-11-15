package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
	"pingtai/internal/config"
	"pingtai/internal/database"
	"pingtai/internal/repository"
	authservice "pingtai/internal/service/auth"
)

func runCreateAdmin(cfg config.Config, args []string) error {
	flags := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	username := flags.String("username", "admin", "administrator username")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if cfg.DBDSN == "" {
		return errors.New("MYSQL_DSN or DATABASE_URL is required")
	}
	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("set ADMIN_PASSWORD or run create-admin in an interactive terminal")
		}
		fmt.Print("New administrator password: ")
		first, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return err
		}
		fmt.Print("Confirm password: ")
		second, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return err
		}
		if string(first) != string(second) {
			return errors.New("passwords do not match")
		}
		password = string(first)
	}
	if len(password) < 8 {
		return errors.New("administrator password must contain at least 8 characters")
	}
	if strings.TrimSpace(*username) == "" {
		return errors.New("administrator username cannot be empty")
	}

	hash, err := authservice.HashPassword(password)
	if err != nil {
		return err
	}
	ctx := context.Background()
	db, err := database.Open(ctx, cfg.DBDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		return err
	}
	users := repository.NewUserRepository(db)
	if err := users.CreateOrUpdateAdmin(ctx, strings.TrimSpace(*username), hash); err != nil {
		return err
	}
	fmt.Printf("Administrator %q is ready.\n", strings.TrimSpace(*username))
	return nil
}
