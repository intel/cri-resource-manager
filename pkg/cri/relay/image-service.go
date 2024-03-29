// Copyright 2019 Intel Corporation. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package relay

import (
	"context"

	criv1 "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (r *relay) ListImages(ctx context.Context,
	req *criv1.ListImagesRequest) (*criv1.ListImagesResponse, error) {
	return r.client.ListImages(ctx, req)
}

func (r *relay) ImageStatus(ctx context.Context,
	req *criv1.ImageStatusRequest) (*criv1.ImageStatusResponse, error) {
	return r.client.ImageStatus(ctx, req)
}

func (r *relay) PullImage(ctx context.Context,
	req *criv1.PullImageRequest) (*criv1.PullImageResponse, error) {
	return r.client.PullImage(ctx, req)
}

func (r *relay) RemoveImage(ctx context.Context,
	req *criv1.RemoveImageRequest) (*criv1.RemoveImageResponse, error) {
	return r.client.RemoveImage(ctx, req)
}

func (r *relay) ImageFsInfo(ctx context.Context,
	req *criv1.ImageFsInfoRequest) (*criv1.ImageFsInfoResponse, error) {
	return r.client.ImageFsInfo(ctx, req)
}
