// Copyright 2022 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package restore

import (
	"context"
	"fmt"
	"testing"

	"github.com/docker/go-units"
	"github.com/pingcap/tidb/br/pkg/lightning/checkpoints"
	"github.com/pingcap/tidb/br/pkg/lightning/config"
	"github.com/pingcap/tidb/br/pkg/lightning/log"
	"github.com/pingcap/tidb/br/pkg/lightning/restore/mock"
	"github.com/stretchr/testify/suite"
)

type precheckImplSuite struct {
	suite.Suite
	cfg           *config.Config
	mockSrc       *mock.MockImportSource
	mockTarget    *mock.MockTargetInfo
	preInfoGetter PreRestoreInfoGetter
}

func TestPrecheckImplSuite(t *testing.T) {
	suite.Run(t, new(precheckImplSuite))
}

func (s *precheckImplSuite) SetupSuite() {
	cfg := &log.Config{}
	cfg.Adjust()
	log.InitLogger(cfg, "debug")
}

func (s *precheckImplSuite) SetupTest() {
	var err error
	s.Require().NoError(err)
	s.mockTarget = mock.NewMockTargetInfo()
	s.cfg = config.NewConfig()
	s.cfg.TikvImporter.Backend = config.BackendLocal
	s.Require().NoError(s.setMockImportData(nil))

}

func (s *precheckImplSuite) setMockImportData(mockDataMap map[string]*mock.MockDBSourceData) error {
	var err error
	s.mockSrc, err = mock.NewMockImportSource(mockDataMap)
	if err != nil {
		return err
	}
	s.preInfoGetter, err = NewPreRestoreInfoGetter(s.cfg, s.mockSrc.GetAllDBFileMetas(), s.mockSrc.GetStorage(), s.mockTarget, nil, nil)
	if err != nil {
		return err
	}
	return nil
}

func (s *precheckImplSuite) generateMockData(
	dbCount int,
	eachDBTableCount int,
	eachTableFileCount int,
	createSchemaSQLFunc func(dbName string, tblName string) string,
	sizeAndDataAndSuffixFunc func(dbID int, tblID int, fileID int) ([]byte, int, string),
) map[string]*mock.MockDBSourceData {
	result := make(map[string]*mock.MockDBSourceData)
	for dbID := 0; dbID < dbCount; dbID++ {
		dbName := fmt.Sprintf("db%d", dbID+1)
		tables := make(map[string]*mock.MockTableSourceData)
		for tblID := 0; tblID < eachDBTableCount; tblID++ {
			tblName := fmt.Sprintf("tbl%d", tblID+1)
			files := []*mock.MockSourceFile{}
			for fileID := 0; fileID < eachTableFileCount; fileID++ {
				fileData, totalSize, suffix := sizeAndDataAndSuffixFunc(dbID, tblID, fileID)
				mockSrcFile := &mock.MockSourceFile{
					FileName:  fmt.Sprintf("/%s/%s/data.%d.%s", dbName, tblName, fileID+1, suffix),
					Data:      fileData,
					TotalSize: totalSize,
				}
				files = append(files, mockSrcFile)
			}
			mockTblSrcData := &mock.MockTableSourceData{
				DBName:    dbName,
				TableName: tblName,
				SchemaFile: &mock.MockSourceFile{
					FileName: fmt.Sprintf("/%s/%s/%s.schema.sql", dbName, tblName, tblName),
					Data:     []byte(createSchemaSQLFunc(dbName, tblName)),
				},
				DataFiles: files,
			}
			tables[tblName] = mockTblSrcData
		}
		mockDBSrcData := &mock.MockDBSourceData{
			Name:   dbName,
			Tables: tables,
		}
		result[dbName] = mockDBSrcData
	}
	return result
}

func (s *precheckImplSuite) TestClusterResourceCheckBasic() {
	var (
		err    error
		ci     PrecheckItem
		result *CheckResult
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ci = NewClusterResourceCheckItem(s.preInfoGetter)
	s.Require().Equal(CheckTargetClusterSize, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().True(result.Passed)

	testMockSrcData := s.generateMockData(1, 1, 1,
		func(dbName string, tblName string) string {
			return fmt.Sprintf("CREATE TABLE %s.%s ( id INTEGER PRIMARY KEY );", dbName, tblName)
		},
		func(dbID int, tblID int, fileID int) ([]byte, int, string) {
			return []byte(nil), 100, "csv"
		},
	)
	s.Require().NoError(s.setMockImportData(testMockSrcData))
	ci = NewClusterResourceCheckItem(s.preInfoGetter)
	s.Require().Equal(CheckTargetClusterSize, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().False(result.Passed)

	s.mockTarget.StorageInfos = append(s.mockTarget.StorageInfos, mock.StorageInfo{
		TotalSize:     1000,
		UsedSize:      100,
		AvailableSize: 900,
	})
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(CheckTargetClusterSize, result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().True(result.Passed)
}

func (s *precheckImplSuite) TestClusterVersionCheckBasic() {
	var (
		err    error
		ci     PrecheckItem
		result *CheckResult
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ci = NewClusterVersionCheckItem(s.preInfoGetter, s.mockSrc.GetAllDBFileMetas())
	s.Require().Equal(CheckTargetClusterVersion, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().True(result.Passed)
}

func (s *precheckImplSuite) TestEmptyRegionCheckBasic() {
	var (
		err    error
		ci     PrecheckItem
		result *CheckResult
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ci = NewEmptyRegionCheckItem(s.preInfoGetter, s.mockSrc.GetAllDBFileMetas())
	s.Require().Equal(CheckTargetClusterEmptyRegion, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().True(result.Passed)

	s.mockTarget.StorageInfos = append(s.mockTarget.StorageInfos,
		mock.StorageInfo{
			TotalSize:     1000,
			UsedSize:      100,
			AvailableSize: 900,
		},
		mock.StorageInfo{
			TotalSize:     1000,
			UsedSize:      100,
			AvailableSize: 900,
		},
	)
	s.mockTarget.EmptyRegionCountMap = map[uint64]int{
		1: 5000,
	}
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(CheckTargetClusterEmptyRegion, result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().False(result.Passed)
}

func (s *precheckImplSuite) TestRegionDistributionCheckBasic() {
	var (
		err    error
		ci     PrecheckItem
		result *CheckResult
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ci = NewRegionDistributionCheckItem(s.preInfoGetter, s.mockSrc.GetAllDBFileMetas())
	s.Require().Equal(CheckTargetClusterRegionDist, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().True(result.Passed)

	s.mockTarget.StorageInfos = append(s.mockTarget.StorageInfos,
		mock.StorageInfo{
			TotalSize:     1000,
			UsedSize:      100,
			AvailableSize: 900,
			RegionCount:   5000,
		},
		mock.StorageInfo{
			TotalSize:     1000,
			UsedSize:      100,
			AvailableSize: 900,
			RegionCount:   500,
		},
	)
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(CheckTargetClusterRegionDist, result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().False(result.Passed)
}

func (s *precheckImplSuite) TestStoragePermissionCheckBasic() {
	var (
		err    error
		ci     PrecheckItem
		result *CheckResult
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.cfg.Mydumper.SourceDir = "file:///tmp"
	ci = NewStoragePermissionCheckItem(s.cfg)
	s.Require().Equal(CheckSourcePermission, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().True(result.Passed)

	s.cfg.Mydumper.SourceDir = "s3://DUMMY-BUCKET/FAKE-DIR/FAKE-DIR2"
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(CheckSourcePermission, result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().False(result.Passed)
}

func (s *precheckImplSuite) TestLargeFileCheckBasic() {
	var (
		err    error
		ci     PrecheckItem
		result *CheckResult
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ci = NewLargeFileCheckItem(s.cfg, s.mockSrc.GetAllDBFileMetas())
	s.Require().Equal(CheckLargeDataFile, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().True(result.Passed)

	testMockSrcData := s.generateMockData(1, 1, 1,
		func(dbName string, tblName string) string {
			return fmt.Sprintf("CREATE TABLE %s.%s ( id INTEGER PRIMARY KEY );", dbName, tblName)
		},
		func(dbID int, tblID int, fileID int) ([]byte, int, string) {
			return []byte(nil), 20 * units.GB, "csv"
		},
	)
	s.Require().NoError(s.setMockImportData(testMockSrcData))
	ci = NewLargeFileCheckItem(s.cfg, s.mockSrc.GetAllDBFileMetas())
	s.Require().Equal(CheckLargeDataFile, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().False(result.Passed)
}

func (s *precheckImplSuite) TestLocalDiskPlacementCheckBasic() {
	var (
		err    error
		ci     PrecheckItem
		result *CheckResult
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.cfg.Mydumper.SourceDir = "file:///dev/"
	s.cfg.TikvImporter.SortedKVDir = "/tmp/"
	ci = NewLocalDiskPlacementCheckItem(s.cfg)
	s.Require().Equal(CheckLocalDiskPlacement, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().True(result.Passed)

	s.cfg.Mydumper.SourceDir = "file:///tmp/"
	s.cfg.TikvImporter.SortedKVDir = "/tmp/"
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(CheckLocalDiskPlacement, result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().False(result.Passed)
}

func (s *precheckImplSuite) TestLocalTempKVDirCheckBasic() {
	var (
		err    error
		ci     PrecheckItem
		result *CheckResult
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.cfg.TikvImporter.SortedKVDir = "/tmp/"
	ci = NewLocalTempKVDirCheckItem(s.cfg, s.preInfoGetter)
	s.Require().Equal(CheckLocalTempKVDir, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().True(result.Passed)

	testMockSrcData := s.generateMockData(1, 1, 1,
		func(dbName string, tblName string) string {
			return fmt.Sprintf("CREATE TABLE %s.%s ( id INTEGER PRIMARY KEY );", dbName, tblName)
		},
		func(dbID int, tblID int, fileID int) ([]byte, int, string) {
			return []byte(nil), 10 * units.TB, "csv"
		},
	)
	s.Require().NoError(s.setMockImportData(testMockSrcData))
	ci = NewLocalTempKVDirCheckItem(s.cfg, s.preInfoGetter)
	s.Require().Equal(CheckLocalTempKVDir, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().False(result.Passed)
}

func (s *precheckImplSuite) TestCheckpointCheckBasic() {
	var (
		err    error
		ci     PrecheckItem
		result *CheckResult
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cpdb := checkpoints.NewNullCheckpointsDB()
	s.cfg.Checkpoint.Enable = true
	ci = NewCheckpointCheckItem(s.cfg, s.preInfoGetter, s.mockSrc.GetAllDBFileMetas(), cpdb)
	s.Require().Equal(CheckCheckpoints, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().True(result.Passed)
}

func (s *precheckImplSuite) TestSchemaCheckBasic() {
	var (
		err    error
		ci     PrecheckItem
		result *CheckResult
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.cfg.Mydumper.CSV.Header = true

	const testCSVData01 string = `ival,sval
111,"aaa"
222,"bbb"
`
	const testCSVData02 string = `xval,sval
111,"aaa"
222,"bbb"
`
	testMockSrcData := s.generateMockData(1, 1, 1,
		func(dbName string, tblName string) string {
			return fmt.Sprintf("CREATE TABLE %s.%s ( id INTEGER PRIMARY KEY AUTO_INCREMENT, ival INTEGER, sval VARCHAR(64) );", dbName, tblName)
		},
		func(dbID int, tblID int, fileID int) ([]byte, int, string) {
			return []byte(testCSVData01), 100, "csv"
		},
	)
	s.Require().NoError(s.setMockImportData(testMockSrcData))
	ci = NewSchemaCheckItem(s.cfg, s.preInfoGetter, s.mockSrc.GetAllDBFileMetas(), nil)
	s.Require().Equal(CheckSourceSchemaValid, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().True(result.Passed)

	testMockSrcData = s.generateMockData(1, 1, 1,
		func(dbName string, tblName string) string {
			return fmt.Sprintf("CREATE TABLE %s.%s ( id INTEGER PRIMARY KEY AUTO_INCREMENT, ival INTEGER NOT NULL, sval VARCHAR(64) NOT NULL);", dbName, tblName)
		},
		func(dbID int, tblID int, fileID int) ([]byte, int, string) {
			return []byte(testCSVData02), 100, "csv"
		},
	)
	s.Require().NoError(s.setMockImportData(testMockSrcData))
	ci = NewSchemaCheckItem(s.cfg, s.preInfoGetter, s.mockSrc.GetAllDBFileMetas(), nil)
	s.Require().Equal(CheckSourceSchemaValid, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().False(result.Passed)
}

func (s *precheckImplSuite) TestCSVHeaderCheckBasic() {
	var (
		err    error
		ci     PrecheckItem
		result *CheckResult
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.cfg.Mydumper.CSV.Header = false

	const testCSVData01 string = `111,"aaa"
222,"bbb"
`
	const testCSVData02 string = `ival,sval
111,"aaa"
222,"bbb"
`
	testMockSrcData := s.generateMockData(1, 1, 1,
		func(dbName string, tblName string) string {
			return fmt.Sprintf("CREATE TABLE %s.%s ( id INTEGER PRIMARY KEY AUTO_INCREMENT, ival INTEGER, sval VARCHAR(64) );", dbName, tblName)
		},
		func(dbID int, tblID int, fileID int) ([]byte, int, string) {
			return []byte(testCSVData01), 100, "csv"
		},
	)
	s.Require().NoError(s.setMockImportData(testMockSrcData))
	ci = NewCSVHeaderCheckItem(s.cfg, s.preInfoGetter, s.mockSrc.GetAllDBFileMetas())
	s.Require().Equal(CheckCSVHeader, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().True(result.Passed)

	testMockSrcData = s.generateMockData(1, 1, 1,
		func(dbName string, tblName string) string {
			return fmt.Sprintf("CREATE TABLE %s.%s ( id INTEGER PRIMARY KEY AUTO_INCREMENT, ival INTEGER, sval VARCHAR(64) );", dbName, tblName)
		},
		func(dbID int, tblID int, fileID int) ([]byte, int, string) {
			return []byte(testCSVData02), 100, "csv"
		},
	)
	s.Require().NoError(s.setMockImportData(testMockSrcData))
	ci = NewCSVHeaderCheckItem(s.cfg, s.preInfoGetter, s.mockSrc.GetAllDBFileMetas())
	s.Require().Equal(CheckCSVHeader, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().False(result.Passed)
}

func (s *precheckImplSuite) TestTableEmptyCheckBasic() {
	var (
		err    error
		ci     PrecheckItem
		result *CheckResult
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testMockSrcData := s.generateMockData(1, 1, 1,
		func(dbName string, tblName string) string {
			return fmt.Sprintf("CREATE TABLE %s.%s ( id INTEGER PRIMARY KEY AUTO_INCREMENT, ival INTEGER, sval VARCHAR(64) );", dbName, tblName)
		},
		func(dbID int, tblID int, fileID int) ([]byte, int, string) {
			return []byte(nil), 100, "csv"
		},
	)
	s.Require().NoError(s.setMockImportData(testMockSrcData))
	ci = NewTableEmptyCheckItem(s.cfg, s.preInfoGetter, s.mockSrc.GetAllDBFileMetas(), nil)
	s.Require().Equal(CheckTargetTableEmpty, ci.GetCheckItemID())
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(ci.GetCheckItemID(), result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().True(result.Passed)

	s.mockTarget.SetTableInfo("db1", "tbl1", &mock.MockTableInfo{
		RowCount: 100,
	})
	result, err = ci.Check(ctx)
	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Require().Equal(CheckTargetTableEmpty, result.Item)
	s.T().Logf("check result message: %s", result.Message)
	s.Require().False(result.Passed)
}
