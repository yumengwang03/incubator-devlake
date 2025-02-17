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
	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/models/domainlayer"
	"github.com/apache/incubator-devlake/core/models/domainlayer/code"
	"github.com/apache/incubator-devlake/core/models/domainlayer/didgen"
	plugin "github.com/apache/incubator-devlake/core/plugin"
	"github.com/apache/incubator-devlake/helpers/pluginhelper/api"
	"github.com/apache/incubator-devlake/plugins/bitbucket/models"
	"reflect"
)

var ConvertPullRequestsMeta = plugin.SubTaskMeta{
	Name:             "convertPullRequests",
	EntryPoint:       ConvertPullRequests,
	EnabledByDefault: true,
	Required:         true,
	Description:      "ConvertPullRequests data from Bitbucket api",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_CODE_REVIEW},
}

func ConvertPullRequests(taskCtx plugin.SubTaskContext) errors.Error {
	db := taskCtx.GetDal()
	data := taskCtx.GetData().(*BitbucketTaskData)
	repoId := data.Repo.BitbucketId

	cursor, err := db.Cursor(
		dal.From(&models.BitbucketPullRequest{}),
		dal.Where("repo_id = ? and connection_id = ?", repoId, data.Options.ConnectionId),
	)
	if err != nil {
		return err
	}
	defer cursor.Close()

	prIdGen := didgen.NewDomainIdGenerator(&models.BitbucketPullRequest{})
	repoIdGen := didgen.NewDomainIdGenerator(&models.BitbucketRepo{})
	domainUserIdGen := didgen.NewDomainIdGenerator(&models.BitbucketAccount{})

	converter, err := api.NewDataConverter(api.DataConverterArgs{
		InputRowType: reflect.TypeOf(models.BitbucketPullRequest{}),
		Input:        cursor,
		RawDataSubTaskArgs: api.RawDataSubTaskArgs{
			Ctx: taskCtx,
			Params: BitbucketApiParams{
				ConnectionId: data.Options.ConnectionId,
				Owner:        data.Options.Owner,
				Repo:         data.Options.Repo,
			},
			Table: RAW_PULL_REQUEST_TABLE,
		},
		Convert: func(inputRow interface{}) ([]interface{}, errors.Error) {
			pr := inputRow.(*models.BitbucketPullRequest)

			// Getting the merge reference commit
			mergeCommit := &models.BitbucketCommit{}
			err = db.First(mergeCommit, dal.Where("LEFT(sha, 12) = ?", pr.MergeCommitSha))
			if err == nil {
				// Setting the PR merged datetime to the commit commited datetime
				pr.MergedAt = &mergeCommit.CommittedDate
			}

			domainPr := &code.PullRequest{
				DomainEntity: domainlayer.DomainEntity{
					Id: prIdGen.Generate(data.Options.ConnectionId, pr.BitbucketId),
				},
				BaseRepoId:     repoIdGen.Generate(data.Options.ConnectionId, pr.BaseRepoId),
				HeadRepoId:     repoIdGen.Generate(data.Options.ConnectionId, pr.HeadRepoId),
				Status:         pr.State,
				Title:          pr.Title,
				Url:            pr.Url,
				AuthorId:       domainUserIdGen.Generate(data.Options.ConnectionId, pr.AuthorId),
				AuthorName:     pr.AuthorName,
				Description:    pr.Description,
				CreatedDate:    pr.BitbucketCreatedAt,
				MergedDate:     pr.MergedAt,
				ClosedDate:     pr.ClosedAt,
				PullRequestKey: pr.Number,
				Type:           pr.Type,
				Component:      pr.Component,
				MergeCommitSha: pr.MergeCommitSha,
				BaseRef:        pr.BaseRef,
				BaseCommitSha:  pr.BaseCommitSha,
				HeadRef:        pr.HeadRef,
				HeadCommitSha:  pr.HeadCommitSha,
			}
			return []interface{}{
				domainPr,
			}, nil
		},
	})
	if err != nil {
		return err
	}

	return converter.Execute()
}
