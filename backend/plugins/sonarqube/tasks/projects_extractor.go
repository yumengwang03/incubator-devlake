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

package tasks

import (
	"encoding/json"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/plugin"
	helper "github.com/apache/incubator-devlake/helpers/pluginhelper/api"
	"github.com/apache/incubator-devlake/plugins/sonarqube/models"
)

var _ plugin.SubTaskEntryPoint = ExtractProjects

func ExtractProjects(taskCtx plugin.SubTaskContext) errors.Error {
	// As we need to assign data.LastAnalysisDate, we can not use CreateRawDataSubTaskArgs
	data := taskCtx.GetData().(*SonarqubeTaskData)
	var params = SonarqubeApiParams{
		ConnectionId: data.Options.ConnectionId,
		ProjectKey:   data.Options.ProjectKey,
	}
	rawDataSubTaskArgs := &helper.RawDataSubTaskArgs{
		Ctx:    taskCtx,
		Params: params,
		Table:  RAW_PROJECTS_TABLE,
	}
	extractor, err := helper.NewApiExtractor(helper.ApiExtractorArgs{
		RawDataSubTaskArgs: *rawDataSubTaskArgs,
		Extract: func(resData *helper.RawData) ([]interface{}, errors.Error) {
			var res struct {
				ProjectKey       string              `json:"key"`
				Name             string              `json:"name"`
				Qualifier        string              `json:"qualifier"`
				Visibility       string              `json:"visibility"`
				LastAnalysisDate *helper.Iso8601Time `json:"lastAnalysisDate"`
				Revision         string              `json:"revision"`
			}
			err := errors.Convert(json.Unmarshal(resData.Data, &res))
			body := &models.SonarqubeProject{
				ConnectionId:     data.Options.ConnectionId,
				ProjectKey:       res.ProjectKey,
				Name:             res.Name,
				Qualifier:        res.Qualifier,
				Visibility:       res.Visibility,
				LastAnalysisDate: res.LastAnalysisDate,
				Revision:         res.Revision,
			}
			data.LastAnalysisDate = body.LastAnalysisDate.ToNullableTime()
			if err != nil {
				return nil, err
			}
			return []interface{}{body}, nil
		},
	})
	if err != nil {
		return err
	}

	return extractor.Execute()
}

var ExtractProjectsMeta = plugin.SubTaskMeta{
	Name:             "ExtractProjects",
	EntryPoint:       ExtractProjects,
	EnabledByDefault: true,
	Description:      "Extract raw data into tool layer table sonarqube_projects",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_SECURITY_TESTING},
}
