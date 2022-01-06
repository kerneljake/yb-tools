/*
Copyright © 2021 Yugabyte Support

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package xcluster

import (
	"bytes"
	fmt "fmt"

	. "github.com/icza/gox/gox"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/yugabyte/yb-tools/yugatool/api/yb/common"
	"github.com/yugabyte/yb-tools/yugatool/api/yb/master"
	"github.com/yugabyte/yb-tools/yugatool/pkg/cmdutil"
	"github.com/yugabyte/yb-tools/yugatool/pkg/util"
)

func InitConsumerCmd(ctx *cmdutil.YugatoolContext) *cobra.Command {
	options := &InitConsumerOptions{}
	cmd := &cobra.Command{
		Use:   "init_consumer",
		Short: "Generate commands to init xCluster replication",
		Long:  `Generate commands to init xCluster replication`,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := ctx.WithCmd(cmd).WithOptions(options).Setup()
			if err != nil {
				return err
			}
			defer ctx.Client.Close()

			return runInitConsumer(ctx, options)
		},
	}
	options.AddFlags(cmd)

	return cmd
}

type InitConsumerOptions struct {
	KeyspaceName string `mapstructure:"keyspace"`
}

func (o *InitConsumerOptions) AddFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.StringVar(&o.KeyspaceName, "keyspace", "", "keyspace to replicate")
}

func (o *InitConsumerOptions) Validate() error {
	return nil
}

var _ cmdutil.CommandOptions = &InitConsumerOptions{}

func runInitConsumer(ctx *cmdutil.YugatoolContext, options *InitConsumerOptions) error {
	tables, err := getTablesToBootstrap(ctx, options.KeyspaceName)
	if err != nil {
		return err
	}

	clusterInfoCmd, err := ctx.Client.Master.MasterService.GetMasterClusterConfig(&master.GetMasterClusterConfigRequestPB{})
	if err != nil {
		return err
	}

	var initCDCCommand bytes.Buffer

	initCDCCommand.WriteString("yb-admin -master_addresses ")
	initCDCCommand.WriteString("$CONSUMER_MASTERS")

	initCDCCommand.WriteRune(' ')

	if util.HasTLS(ctx.Client.Config.GetTlsOpts()) {
		initCDCCommand.WriteString("-certs_dir_name $CERTS_DIR ")
	}

	initCDCCommand.WriteString("setup_universe_replication ")

	initCDCCommand.WriteString(clusterInfoCmd.ClusterConfig.GetClusterUuid())

	initCDCCommand.WriteString(" $PRODUCER_MASTERS ")
	for i, table := range tables.GetTables() {
		if table.TableType.Number() == common.TableType_YQL_TABLE_TYPE.Number() {
			initCDCCommand.Write(table.GetId())
			if i+1 < len(tables.GetTables()) {
				initCDCCommand.WriteRune(',')
			}
		}
	}

	initCDCCommand.WriteRune(' ')

	for i, table := range tables.GetTables() {
		if table.TableType.Number() == common.TableType_YQL_TABLE_TYPE.Number() {
			streams, err := ctx.Client.Master.MasterService.ListCDCStreams(&master.ListCDCStreamsRequestPB{
				TableId: NewString(string(table.GetId())),
			})
			if err != nil {
				return err
			}
			if streams.Error != nil {
				return errors.Errorf("error getting stream table %s: %s", table, streams.Error)
			}
			if len(streams.GetStreams()) != 1 {
				return errors.Errorf("found too many streams for table %s: %s", table, streams)
			}

			initCDCCommand.Write(streams.Streams[0].StreamId)
			if i+1 < len(tables.GetTables()) {
				initCDCCommand.WriteRune(',')
			}
		}
	}

	fmt.Println(initCDCCommand.String())
	return nil
}
