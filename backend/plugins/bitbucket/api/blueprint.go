/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/models/domainlayer/didgen"
	"github.com/apache/incubator-devlake/core/plugin"
	"github.com/apache/incubator-devlake/core/utils"
	"github.com/apache/incubator-devlake/helpers/pluginhelper/api"
	aha "github.com/apache/incubator-devlake/helpers/pluginhelper/api/apihelperabstract"
	"github.com/apache/incubator-devlake/plugins/bitbucket/models"
	"github.com/apache/incubator-devlake/plugins/bitbucket/tasks"
)

func MakePipelinePlan(subtaskMetas []plugin.SubTaskMeta, connectionId uint64, scope []*plugin.BlueprintScopeV100) (plugin.PipelinePlan, errors.Error) {
	connection := new(models.BitbucketConnection)
	err := connectionHelper.FirstById(connection, connectionId)
	if err != nil {
		return nil, err
	}
	apiClient, err := api.NewApiClientFromConnection(context.TODO(), basicRes, connection)
	if err != nil {
		return nil, err
	}
	return makePipelinePlan(subtaskMetas, scope, apiClient, connection)
}

func makePipelinePlan(
	subtaskMetas []plugin.SubTaskMeta,
	scope []*plugin.BlueprintScopeV100,
	apiClient aha.ApiClientAbstract,
	connection *models.BitbucketConnection,
) (plugin.PipelinePlan, errors.Error) {
	var err errors.Error
	plan := make(plugin.PipelinePlan, len(scope))
	var repo *tasks.BitbucketApiRepo
	for i, scopeElem := range scope {
		// handle taskOptions and transformationRules, by dumping them to taskOptions
		transformationRules := make(map[string]interface{})
		if len(scopeElem.Transformation) > 0 {
			err = errors.Convert(json.Unmarshal(scopeElem.Transformation, &transformationRules))
			if err != nil {
				return nil, err
			}
		}
		// refdiff
		if refdiffRules, ok := transformationRules["refdiff"]; ok && refdiffRules != nil {
			// add a new task to next stage
			j := i + 1
			if j == len(plan) {
				plan = append(plan, nil)
			}
			plan[j] = plugin.PipelineStage{
				{
					Plugin:  "refdiff",
					Options: refdiffRules.(map[string]interface{}),
				},
			}
			// remove it from bitbucket transformationRules
			delete(transformationRules, "refdiff")
		}
		// construct task options for bitbucket
		options := make(map[string]interface{})
		err = errors.Convert(json.Unmarshal(scopeElem.Options, &options))
		if err != nil {
			return nil, err
		}
		options["connectionId"] = connection.ID
		options["transformationRules"] = transformationRules
		// make sure task options is valid
		op, err := tasks.DecodeAndValidateTaskOptions(options)
		if err != nil {
			return nil, err
		}
		memorizedGetApiRepo := func() (*tasks.BitbucketApiRepo, errors.Error) {
			if repo == nil {
				repo, err = getApiRepo(op, apiClient)
			}
			return repo, err
		}
		// construct subtasks
		subtasks, err := api.MakePipelinePlanSubtasks(subtaskMetas, scopeElem.Entities)
		if err != nil {
			return nil, err
		}
		stage := plan[i]
		if stage == nil {
			stage = plugin.PipelineStage{}
		}
		stage = append(stage, &plugin.PipelineTask{
			Plugin:   "bitbucket",
			Subtasks: subtasks,
			Options:  options,
		})
		// collect git data by gitextractor if CODE was requested
		if utils.StringsContains(scopeElem.Entities, plugin.DOMAIN_TYPE_CODE) {
			repo, err = memorizedGetApiRepo()
			if err != nil {
				return nil, err
			}
			originalUrl := ""
			for _, u := range repo.Links.Clone {
				if u.Name == "https" {
					originalUrl = u.Href
				}
			}
			cloneUrl, err := errors.Convert01(url.Parse(originalUrl))
			if err != nil {
				return nil, err
			}
			cloneUrl.User = url.UserPassword(connection.Username, connection.Password)
			stage = append(stage, &plugin.PipelineTask{
				Plugin: "gitextractor",
				Options: map[string]interface{}{
					"url":    cloneUrl.String(),
					"repoId": didgen.NewDomainIdGenerator(&models.BitbucketRepo{}).Generate(connection.ID, fmt.Sprintf("%s/%s", op.Owner, op.Repo)),
				},
			})

		}
		plan[i] = stage
		repo = nil
	}
	return plan, nil
}

func getApiRepo(
	op *tasks.BitbucketOptions,
	apiClient aha.ApiClientAbstract,
) (*tasks.BitbucketApiRepo, errors.Error) {
	res, err := apiClient.Get(path.Join("repositories", op.Owner, op.Repo), nil, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.Default.New(fmt.Sprintf(
			"unexpected status code when requesting repo detail %d %s",
			res.StatusCode, res.Request.URL.String(),
		))
	}
	body, err := errors.Convert01(io.ReadAll(res.Body))
	if err != nil {
		return nil, err
	}
	apiRepo := new(tasks.BitbucketApiRepo)
	err = errors.Convert(json.Unmarshal(body, apiRepo))
	if err != nil {
		return nil, err
	}
	for _, u := range apiRepo.Links.Clone {
		if u.Name == "https" {
			return apiRepo, nil
		}
	}
	return nil, errors.Default.New("no clone url")
}
