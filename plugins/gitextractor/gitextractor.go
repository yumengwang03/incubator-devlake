package main

import (
	"context"
	"errors"
	"strings"

	"github.com/merico-dev/lake/logger"
	lakeModels "github.com/merico-dev/lake/models"
	"github.com/merico-dev/lake/plugins/core"
	"github.com/merico-dev/lake/plugins/gitextractor/parser"
	"github.com/merico-dev/lake/plugins/gitextractor/store"
	"github.com/mitchellh/mapstructure"
)

type GitExtractorOptions struct {
	RepoId     string `json:"repoId"`
	Url        string `json:"url"`
	User       string `json:"user"`
	Password   string `json:"password"`
	PrivateKey string `json:"privateKey"`
	Passphrase string `json:"passphrase"`
	Proxy      string `json:"proxy"`
}

func (o GitExtractorOptions) Valid() error {
	if o.RepoId == "" {
		return errors.New("empty repoId")
	}
	if o.Url == "" {
		return errors.New("empty url")
	}
	url := strings.TrimPrefix(o.Url, "ssh://")
	if !(strings.HasPrefix(o.Url, "http") || strings.HasPrefix(url, "git@") || strings.HasPrefix(o.Url, "/")) {
		return errors.New("wrong url")
	}
	if o.Proxy != "" && !strings.HasPrefix(o.Proxy, "http://") {
		return errors.New("only support http proxy")
	}
	return nil
}

type GitExtractor struct{}

func (plugin GitExtractor) Description() string {
	return "extract infos from git repository"
}

func (plugin GitExtractor) Init() {
	logger.Info("INFO >>> init git extractor", true)
}

func (plugin GitExtractor) Execute(options map[string]interface{}, progress chan<- float32, ctx context.Context) error {
	logger.Print("start gitlab plugin execution")
	var op GitExtractorOptions
	err := mapstructure.Decode(options, &op)
	if err != nil {
		return err
	}
	err = op.Valid()
	if err != nil {
		return err
	}
	storage := store.NewDatabase(lakeModels.Db)
	defer storage.Close()
	progress <- 0.1
	p := parser.NewLibGit2(storage)
	if strings.HasPrefix(op.Url, "http") {
		err = p.CloneOverHTTP(ctx, op.RepoId, op.Url, op.User, op.Password, op.Proxy)
	} else if url := strings.TrimPrefix(op.Url, "ssh://"); strings.HasPrefix(url, "git@") {
		err = p.CloneOverSSH(ctx, op.RepoId, url, op.PrivateKey, op.Passphrase)
	}else if strings.HasPrefix(op.Url, "/") {
		err = p.LocalRepo(ctx, op.Url, op.RepoId)
	}
	if err != nil{
		return err
	}
	progress <- 1
	return nil
}

func (plugin GitExtractor) RootPkgPath() string {
	return "github.com/merico-dev/lake/plugins/gitextractor"
}

func (plugin GitExtractor) ApiResources() map[string]map[string]core.ApiResourceHandler {
	return nil
}

// Export a variable named PluginEntry for Framework to search and load
var PluginEntry GitExtractor //nolint
