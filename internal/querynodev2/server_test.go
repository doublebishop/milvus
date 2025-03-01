// Licensed to the LF AI & Data foundation under one
// or more contributor license agreements. See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership. The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package querynodev2

import (
	"context"
	"os"
	"testing"

	"github.com/cockroachdb/errors"

	"github.com/milvus-io/milvus-proto/go-api/commonpb"
	"github.com/milvus-io/milvus/internal/storage"
	"github.com/milvus-io/milvus/internal/util/dependency"
	"github.com/milvus-io/milvus/internal/util/etcd"
	"github.com/milvus-io/milvus/internal/util/paramtable"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type QueryNodeSuite struct {
	suite.Suite
	// data
	address string

	// dependency
	params              *paramtable.ComponentParam
	node                *QueryNode
	etcd                *clientv3.Client
	chunkManagerFactory *storage.ChunkManagerFactory

	// mock
	factory *dependency.MockFactory
}

func (suite *QueryNodeSuite) SetupSuite() {
	suite.address = "test-address"

}

func (suite *QueryNodeSuite) SetupTest() {
	var err error
	paramtable.Init()
	suite.params = paramtable.Get()
	suite.params.Save(suite.params.QueryNodeCfg.GCEnabled.Key, "false")

	// mock factory
	suite.factory = dependency.NewMockFactory(suite.T())
	suite.chunkManagerFactory = storage.NewChunkManagerFactory("local", storage.RootPath("/tmp/milvus_test"))
	// new node
	suite.node = NewQueryNode(context.Background(), suite.factory)
	// init etcd
	suite.etcd, err = etcd.GetEtcdClient(
		suite.params.EtcdCfg.UseEmbedEtcd.GetAsBool(),
		suite.params.EtcdCfg.EtcdUseSSL.GetAsBool(),
		suite.params.EtcdCfg.Endpoints.GetAsStrings(),
		suite.params.EtcdCfg.EtcdTLSCert.GetValue(),
		suite.params.EtcdCfg.EtcdTLSKey.GetValue(),
		suite.params.EtcdCfg.EtcdTLSCACert.GetValue(),
		suite.params.EtcdCfg.EtcdTLSMinVersion.GetValue())
	suite.NoError(err)
}

func (suite *QueryNodeSuite) TearDownTest() {
	suite.etcd.Close()
	os.RemoveAll("/tmp/milvus-test")
}
func (suite *QueryNodeSuite) TestBasic() {
	// mock expect
	suite.factory.EXPECT().Init(mock.Anything).Return()
	suite.factory.EXPECT().NewPersistentStorageChunkManager(mock.Anything).Return(suite.chunkManagerFactory.NewPersistentStorageChunkManager(context.Background()))

	var err error
	suite.node.SetEtcdClient(suite.etcd)
	err = suite.node.Init()
	suite.NoError(err)

	// node shoule be unhealthy before node start
	suite.False(suite.node.lifetime.GetState() == commonpb.StateCode_Healthy)

	// start node
	err = suite.node.Start()
	suite.NoError(err)

	// node shoule be healthy after node start
	suite.True(suite.node.lifetime.GetState() == commonpb.StateCode_Healthy)

	// register node to etcd
	suite.node.session.TriggerKill = false
	err = suite.node.Register()
	suite.NoError(err)

	// set and get address
	suite.node.SetAddress(suite.address)
	address := suite.node.GetAddress()
	suite.Equal(suite.address, address)

	// close node
	err = suite.node.Stop()
	suite.NoError(err)

	// node should be unhealthy after node stop
	suite.False(suite.node.lifetime.GetState() == commonpb.StateCode_Healthy)
}

func (suite *QueryNodeSuite) TestInit_RemoteChunkManagerFailed() {
	var err error
	suite.node.SetEtcdClient(suite.etcd)

	// init remote chunk manager failed
	suite.factory.EXPECT().Init(mock.Anything).Return()
	suite.factory.EXPECT().NewPersistentStorageChunkManager(mock.Anything).Return(nil, errors.New("mock error"))
	err = suite.node.Init()
	suite.Error(err)
}

func (suite *QueryNodeSuite) TestInit_VactorChunkManagerFailed() {
	var err error
	suite.node.SetEtcdClient(suite.etcd)

	// init vactor chunk manager failed
	suite.factory.EXPECT().Init(mock.Anything).Return()
	suite.factory.EXPECT().NewPersistentStorageChunkManager(mock.Anything).Return(suite.chunkManagerFactory.NewPersistentStorageChunkManager(context.Background())).Once()
	suite.factory.EXPECT().NewPersistentStorageChunkManager(mock.Anything).Return(nil, errors.New("mock error")).Once()
	err = suite.node.Init()
	suite.Error(err)
}

func TestQueryNode(t *testing.T) {
	suite.Run(t, new(QueryNodeSuite))
}
