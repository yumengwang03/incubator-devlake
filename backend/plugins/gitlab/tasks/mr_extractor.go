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
	"regexp"
	"strings"

	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/plugin"
	"github.com/apache/incubator-devlake/helpers/pluginhelper/api"
	"github.com/apache/incubator-devlake/plugins/gitlab/models"
)

type MergeRequestRes struct {
	GitlabId        int `json:"id"`
	Iid             int
	ProjectId       int `json:"project_id"`
	SourceProjectId int `json:"source_project_id"`
	TargetProjectId int `json:"target_project_id"`
	State           string
	Title           string
	Description     string
	WebUrl          string           `json:"web_url"`
	UserNotesCount  int              `json:"user_notes_count"`
	WorkInProgress  bool             `json:"work_in_progress"`
	SourceBranch    string           `json:"source_branch"`
	TargetBranch    string           `json:"target_branch"`
	GitlabCreatedAt api.Iso8601Time  `json:"created_at"`
	MergedAt        *api.Iso8601Time `json:"merged_at"`
	ClosedAt        *api.Iso8601Time `json:"closed_at"`
	MergeCommitSha  string           `json:"merge_commit_sha"`
	MergedBy        struct {
		Username string `json:"username"`
	} `json:"merged_by"`
	Author struct {
		Id       int    `json:"id"`
		Username string `json:"username"`
	}
	Reviewers        []Reviewer
	FirstCommentTime api.Iso8601Time
	Labels           []string `json:"labels"`
}

type Reviewer struct {
	GitlabId       int `json:"id"`
	MergeRequestId int
	Name           string
	Username       string
	State          string
	AvatarUrl      string `json:"avatar_url"`
	WebUrl         string `json:"web_url"`
}

var ExtractApiMergeRequestsMeta = plugin.SubTaskMeta{
	Name:             "extractApiMergeRequests",
	EntryPoint:       ExtractApiMergeRequests,
	EnabledByDefault: true,
	Description:      "Extract raw merge requests data into tool layer table GitlabMergeRequest and GitlabReviewer",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_CODE_REVIEW},
}

func ExtractApiMergeRequests(taskCtx plugin.SubTaskContext) errors.Error {
	rawDataSubTaskArgs, data := CreateRawDataSubTaskArgs(taskCtx, RAW_MERGE_REQUEST_TABLE)
	config := data.Options.GitlabTransformationRule
	var labelTypeRegex *regexp.Regexp
	var labelComponentRegex *regexp.Regexp
	var prType = config.PrType

	var err1 error
	if len(prType) > 0 {
		labelTypeRegex, err1 = regexp.Compile(prType)
		if err1 != nil {
			return errors.Default.Wrap(err1, "regexp Compile prType failed")
		}
	}
	var prComponent = config.PrComponent
	if len(prComponent) > 0 {
		labelComponentRegex, err1 = regexp.Compile(prComponent)
		if err1 != nil {
			return errors.Default.Wrap(err1, "regexp Compile prComponent failed")
		}
	}

	extractor, err := api.NewApiExtractor(api.ApiExtractorArgs{
		RawDataSubTaskArgs: *rawDataSubTaskArgs,
		Extract: func(row *api.RawData) ([]interface{}, errors.Error) {

			mr := &MergeRequestRes{}
			s := string(row.Data)
			err := errors.Convert(json.Unmarshal(row.Data, mr))
			if err != nil {
				return nil, err
			}

			gitlabMergeRequest, err := convertMergeRequest(mr)
			if err != nil {
				return nil, err
			}

			// if we can not find merged_at and closed_at info in the detail
			// we need get detail for gitlab v11
			if !strings.Contains(s, "\"merged_at\":") {
				if !strings.Contains(s, "\"closed_at\":") {
					gitlabMergeRequest.IsDetailRequired = true
				}
			}

			results := make([]interface{}, 0, len(mr.Reviewers)+1)
			gitlabMergeRequest.ConnectionId = data.Options.ConnectionId
			results = append(results, gitlabMergeRequest)
			for _, label := range mr.Labels {
				results = append(results, &models.GitlabMrLabel{
					MrId:         gitlabMergeRequest.GitlabId,
					LabelName:    label,
					ConnectionId: data.Options.ConnectionId,
				})
				// if pr.Type has not been set and prType is set in .env, process the below
				if labelTypeRegex != nil {
					groups := labelTypeRegex.FindStringSubmatch(label)
					if len(groups) > 1 {
						gitlabMergeRequest.Type = groups[1]
					}
				}

				// if pr.Component has not been set and prComponent is set in .env, process
				if labelComponentRegex != nil {
					groups := labelComponentRegex.FindStringSubmatch(label)
					if len(groups) > 1 {
						gitlabMergeRequest.Component = groups[1]
					}
				}
			}
			for _, reviewer := range mr.Reviewers {
				gitlabReviewer := &models.GitlabReviewer{
					ConnectionId:   data.Options.ConnectionId,
					GitlabId:       reviewer.GitlabId,
					MergeRequestId: mr.GitlabId,
					ProjectId:      data.Options.ProjectId,
					Username:       reviewer.Username,
					Name:           reviewer.Name,
					State:          reviewer.State,
					AvatarUrl:      reviewer.AvatarUrl,
					WebUrl:         reviewer.WebUrl,
				}
				results = append(results, gitlabReviewer)
			}

			return results, nil
		},
	})

	if err != nil {
		return errors.Convert(err)
	}

	err = extractor.Execute()
	if err != nil {
		return err
	}

	return nil
}

func convertMergeRequest(mr *MergeRequestRes) (*models.GitlabMergeRequest, errors.Error) {
	gitlabMergeRequest := &models.GitlabMergeRequest{
		GitlabId:         mr.GitlabId,
		Iid:              mr.Iid,
		ProjectId:        mr.ProjectId,
		SourceProjectId:  mr.SourceProjectId,
		TargetProjectId:  mr.TargetProjectId,
		State:            mr.State,
		Title:            mr.Title,
		Description:      mr.Description,
		WebUrl:           mr.WebUrl,
		UserNotesCount:   mr.UserNotesCount,
		WorkInProgress:   mr.WorkInProgress,
		IsDetailRequired: false,
		SourceBranch:     mr.SourceBranch,
		TargetBranch:     mr.TargetBranch,
		MergeCommitSha:   mr.MergeCommitSha,
		MergedAt:         api.Iso8601TimeToTime(mr.MergedAt),
		GitlabCreatedAt:  mr.GitlabCreatedAt.ToTime(),
		ClosedAt:         api.Iso8601TimeToTime(mr.ClosedAt),
		MergedByUsername: mr.MergedBy.Username,
		AuthorUsername:   mr.Author.Username,
		AuthorUserId:     mr.Author.Id,
	}
	return gitlabMergeRequest, nil
}
