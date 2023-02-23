// Copyright 2019 The multi-geth Authors
// This file is part of the multi-geth library.
//
// The multi-geth library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The multi-geth library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the multi-geth library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"bytes"
	"encoding/json"
	"math/big"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/params/confp"
	"github.com/ethereum/go-ethereum/params/types/coregeth"
	"github.com/ethereum/go-ethereum/params/types/ctypes"
	"github.com/ethereum/go-ethereum/params/types/genesisT"
	"github.com/ethereum/go-ethereum/params/types/goethereum"
	"github.com/ethereum/go-ethereum/params/types/multigeth"
	"github.com/ethereum/go-ethereum/params/vars"
	"github.com/ethereum/go-ethereum/trie"
)

func TestSetupGenesisBlock(t *testing.T) {
	db := rawdb.NewMemoryDatabase()

	defaultGenesisBlock := params.DefaultGenesisBlock()

	config, hash, err := SetupGenesisBlock(db, trie.NewDatabase(db), defaultGenesisBlock)
	if err != nil {
		t.Errorf("err: %v", err)
	}
	if wantHash := GenesisToBlock(defaultGenesisBlock, nil).Hash(); wantHash != hash {
		t.Errorf("mismatch block hash, want: %x, got: %x", wantHash, hash)
	}
	if diffs := confp.Equal(reflect.TypeOf((*ctypes.ChainConfigurator)(nil)), defaultGenesisBlock.Config, config); len(diffs) != 0 {
		for _, diff := range diffs {
			t.Error("mismatch", "diff=", diff, "in", defaultGenesisBlock.Config, "out", config)
		}
	}

	classicGenesisBlock := params.DefaultClassicGenesisBlock()

	clConfig, clHash, clErr := SetupGenesisBlock(db, trie.NewDatabase(db), classicGenesisBlock)
	if clErr != nil {
		t.Errorf("err: %v", clErr)
	}
	if wantHash := GenesisToBlock(classicGenesisBlock, nil).Hash(); wantHash != clHash {
		t.Errorf("mismatch block hash, want: %x, got: %x", wantHash, clHash)
	}
	if diffs := confp.Equal(reflect.TypeOf((*ctypes.ChainConfigurator)(nil)), classicGenesisBlock.Config, clConfig); len(diffs) != 0 {
		for _, diff := range diffs {
			t.Error("mismatch", "diff=", diff, "in", classicGenesisBlock.Config, "out", clConfig)
		}
	}
}

func TestInvalidCliqueConfig(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	gspec := params.DefaultGoerliGenesisBlock()
	gspec.ExtraData = []byte{}

	if _, err := CommitGenesis(gspec, db, trie.NewDatabase(db)); err == nil {
		t.Fatal("Expected error on invalid clique config")
	}
}

func TestSetupGenesis(t *testing.T) {
	var (
		customghash = common.HexToHash("0x89c99d90b79719238d2645c7642f2c9295246e80775b38cfd162b696817fbd50")
		customg     = genesisT.Genesis{
			Config: &goethereum.ChainConfig{HomesteadBlock: big.NewInt(3)},
			Alloc: genesisT.GenesisAlloc{
				{1}: {Balance: big.NewInt(1), Storage: map[common.Hash]common.Hash{{1}: {1}}},
			},
		}
		oldcustomg = customg
	)
	oldcustomg.Config = &goethereum.ChainConfig{HomesteadBlock: big.NewInt(2)}
	tests := []struct {
		name       string
		fn         func(ethdb.Database) (ctypes.ChainConfigurator, common.Hash, error)
		wantConfig ctypes.ChainConfigurator
		wantHash   common.Hash
		wantErr    error
	}{
		{
			name: "genesis without ChainConfig",
			fn: func(db ethdb.Database) (ctypes.ChainConfigurator, common.Hash, error) {
				return SetupGenesisBlock(db, trie.NewDatabase(db), new(genesisT.Genesis))
			},
			wantErr:    errGenesisNoConfig,
			wantConfig: params.AllEthashProtocolChanges,
		},
		{
			name: "no block in DB, genesis == nil",
			fn: func(db ethdb.Database) (ctypes.ChainConfigurator, common.Hash, error) {
				return SetupGenesisBlock(db, trie.NewDatabase(db), nil)
			},
			wantHash:   params.MainnetGenesisHash,
			wantConfig: params.MainnetChainConfig,
		},
		{
			name: "mainnet block in DB, genesis == nil",
			fn: func(db ethdb.Database) (ctypes.ChainConfigurator, common.Hash, error) {
				MustCommitGenesis(db, params.DefaultGenesisBlock())
				return SetupGenesisBlock(db, trie.NewDatabase(db), nil)
			},
			wantHash:   params.MainnetGenesisHash,
			wantConfig: params.MainnetChainConfig,
		},
		{
			name: "custom block in DB, genesis == nil",
			fn: func(db ethdb.Database) (ctypes.ChainConfigurator, common.Hash, error) {
				MustCommitGenesis(db, &customg)
				return SetupGenesisBlock(db, trie.NewDatabase(db), nil)
			},
			wantHash:   customghash,
			wantConfig: customg.Config,
		},
		{
			name: "custom block in DB, genesis == goerli",
			fn: func(db ethdb.Database) (ctypes.ChainConfigurator, common.Hash, error) {
				MustCommitGenesis(db, &customg)
				return SetupGenesisBlock(db, trie.NewDatabase(db), params.DefaultGoerliGenesisBlock())
			},
			wantErr:    &genesisT.GenesisMismatchError{Stored: customghash, New: params.GoerliGenesisHash},
			wantHash:   params.GoerliGenesisHash,
			wantConfig: params.GoerliChainConfig,
		},
		{
			name: "compatible config in DB",
			fn: func(db ethdb.Database) (ctypes.ChainConfigurator, common.Hash, error) {
				MustCommitGenesis(db, &oldcustomg)
				return SetupGenesisBlock(db, trie.NewDatabase(db), &customg)
			},
			wantHash:   customghash,
			wantConfig: customg.Config,
		},
		{
			name: "incompatible config in DB",
			fn: func(db ethdb.Database) (ctypes.ChainConfigurator, common.Hash, error) {
				// Commit the 'old' genesis block with Homestead transition at #2.
				// Advance to block #4, past the homestead transition block of customg.
				genesis := MustCommitGenesis(db, &oldcustomg)

				bc, _ := NewBlockChain(db, nil, &oldcustomg, nil, ethash.NewFullFaker(), vm.Config{}, nil, nil)
				defer bc.Stop()

				blocks, _ := GenerateChain(oldcustomg.Config, genesis, ethash.NewFaker(), db, 4, nil)
				bc.InsertChain(blocks)

				// This should return a compatibility error.
				return SetupGenesisBlock(db, trie.NewDatabase(db), &customg)
			},
			wantHash:   customghash,
			wantConfig: customg.Config,
			wantErr: &confp.ConfigCompatError{
				What:          "incompatible fork value: GetEIP2Transition",
				StoredBlock:   big.NewInt(2),
				NewBlock:      big.NewInt(3),
				RewindToBlock: 1,
			},
		},
	}

	for _, test := range tests {

		db := rawdb.NewMemoryDatabase()
		config, hash, err := test.fn(db)
		// Check the return values.
		if !reflect.DeepEqual(err, test.wantErr) {
			spew := spew.ConfigState{DisablePointerAddresses: true, DisableCapacities: true}
			t.Errorf("%s: returned error %#v, want %#v", test.name, spew.NewFormatter(err), spew.NewFormatter(test.wantErr))
		}
		if !reflect.DeepEqual(config, test.wantConfig) {
			t.Errorf("%s:\nreturned %v\nwant     %v", test.name, config, test.wantConfig)
		}
		if hash != test.wantHash {
			t.Errorf("%s: returned hash %s, want %s", test.name, hash.Hex(), test.wantHash.Hex())
		} else if err == nil {
			// Check database content.
			stored := rawdb.ReadBlock(db, test.wantHash, 0)
			if stored.Hash() != test.wantHash {
				t.Errorf("%s: block in DB has hash %s, want %s", test.name, stored.Hash(), test.wantHash)
			}
		}
	}
}

// This test is very similar and in some way redundant to generic.TestUnmarshalChainConfigurator2
// but intended to be more "integrative".
func TestSetupGenesisBlock2(t *testing.T) {
	db := rawdb.NewMemoryDatabase()

	// An example of v1.9.6 multigeth config marshaled to JSON.
	// Note the fields EIP1108FBlock; these were included accidentally because
	// of a typo in the struct field json tags, and because of that, will
	// not be omitted when empty, nor "properly" (lowercase) named.
	//
	// This should be treated as an 'oldmultigeth' data type, since it has values which are
	// not present in the contemporary multigeth data type.
	//
	// In this test we'll assume that this is the config which has been
	// written to the database, and which should be superceded by the
	// config below (cc_v197_a).
	var cc_v196_a = `{
  "chainId": 61,
  "homesteadBlock": 1150000,
  "daoForkBlock": 1920000,
  "eip150Block": 2500000,
  "eip150Hash": "0xca12c63534f565899681965528d536c52cb05b7c48e269c2a6cb77ad864d878a",
  "eip155Block": 3000000,
  "eip158Block": 8772000,
  "byzantiumBlock": 8772000,
  "constantinopleBlock": 9573000,
  "petersburgBlock": 9573000,
  "ethash": {},
  "trustedCheckpoint": null,
  "trustedCheckpointOracle": null,
  "networkId": 1,
  "eip7FBlock": null,
  "eip160Block": 3000000,
  "EIP1108FBlock": null,
  "EIP1344FBlock": null,
  "EIP1884FBlock": null,
  "EIP2028FBlock": null,
  "EIP2200FBlock": null,
  "ecip1010PauseBlock": 3000000,
  "ecip1010Length": 2000000,
  "ecip1017EraBlock": 5000000,
  "disposalBlock": 5900000
}
`

	// An example of a "healthy" multigeth configuration marshaled to JSON.
	var cc_v197_a = `{
    "networkId": 1,
    "chainId": 61,
    "eip2FBlock": 1150000,
    "eip7FBlock": 1150000,
    "eip150Block": 2500000,
    "eip155Block": 3000000,
    "eip160Block": 3000000,
    "eip161FBlock": 8772000,
    "eip170FBlock": 8772000,
    "eip100FBlock": 8772000,
    "eip140FBlock": 8772000,
    "eip198FBlock": 8772000,
    "eip211FBlock": 8772000,
    "eip212FBlock": 8772000,
    "eip213FBlock": 8772000,
    "eip214FBlock": 8772000,
    "eip658FBlock": 8772000,
    "eip145FBlock": 9573000,
    "eip1014FBlock": 9573000,
    "eip1052FBlock": 9573000,
    "eip152FBlock": 10500839,
    "eip1108FBlock": 10500839,
    "eip1344FBlock": 10500839,
    "eip2028FBlock": 10500839,
    "eip2200FBlock": 10500839,
    "ecip1010PauseBlock": 3000000,
    "ecip1010Length": 2000000,
    "ecip1017FBlock": 5000000,
    "ecip1017EraRounds": 5000000,
    "disposalBlock": 5900000,
    "ethash": {},
    "trustedCheckpoint": null,
    "trustedCheckpointOracle": null,
    "requireBlockHashes": {
        "1920000": "0x94365e3a8c0b35089c1d1195081fe7489b528a84b22199c916180db8b28ade7f",
        "2500000": "0xca12c63534f565899681965528d536c52cb05b7c48e269c2a6cb77ad864d878a"
    }
}`
	headHeight := uint64(9700559)
	genHash := common.HexToHash("0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3")
	headHash := common.HexToHash("0xe618c1b2d738dfa09052e199e5870274f09eb83c684a8a2c194b82dedc00a977")

	_, hash, err := SetupGenesisBlock(db, trie.NewDatabase(db), params.DefaultClassicGenesisBlock())
	if err != nil {
		t.Fatal(err)
	}
	if genHash != hash {
		t.Fatal("mismatch genesis hash")
	}
	// Simulate that the stored config is the v1.9.6 version.
	// This skips the marshaling step of the rawdb.WriteChainConfig method,
	// allowing us to just slap this value in there straight.
	err = db.Put(rawdb.ConfigKey(genHash), []byte(cc_v196_a))
	if err != nil {
		t.Fatal(err)
	}

	// First test: show that the config we've stored in the database gets unmarshaled
	// as an 'oldmultigeth' config.
	storedConf := rawdb.ReadChainConfig(db, genHash)
	if storedConf == nil {
		t.Fatal("nil stored conf")
	}
	wantType := reflect.TypeOf(&multigeth.ChainConfig{})
	if reflect.TypeOf(storedConf) != wantType {
		t.Fatalf("mismatch, want: %v, got: %v", wantType, reflect.TypeOf(storedConf))
	}

	// "Fast forward" the database indicators.
	rawdb.WriteHeadHeaderHash(db, headHash)
	rawdb.WriteHeaderNumber(db, headHash, headHeight)

	// Setup genesis again, but now with contemporary chain config, ie v1.9.7+
	conf2, hash2, err := SetupGenesisBlock(db, trie.NewDatabase(db), params.DefaultClassicGenesisBlock())
	if err != nil {
		t.Fatal(err)
	}
	if hash2 != hash {
		t.Fatal("mismatch hash")
	}
	// Test that our setup config return the proper type configurator.
	wantType = reflect.TypeOf(&coregeth.CoreGethChainConfig{})
	if reflect.TypeOf(conf2) != wantType {
		t.Fatalf("mismatch, want: %v, got: %v", wantType, reflect.TypeOf(conf2))
	}

	// Nitty gritty test that the contemporary stored config, when compactly marshaled,
	// is equal to the expected "healthy" variable value set above.
	// Use compaction to remove whitespace considerations.
	outConf := rawdb.ReadChainConfig(db, genHash)
	outConfMarshal, err := json.MarshalIndent(outConf, "", "    ")
	if err != nil {
		t.Fatal(err)
	}

	bCompactB := []byte{}
	bufCompactB := bytes.NewBuffer(bCompactB)

	bCompactA := []byte{}
	bufCompactA := bytes.NewBuffer(bCompactA)

	err = json.Compact(bufCompactB, outConfMarshal)
	if err != nil {
		t.Fatal(err)
	}

	err = json.Compact(bufCompactA, []byte(cc_v197_a))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(bCompactB, bCompactA) {
		t.Fatal("different config")
	}
}

// TestGenesisHashes checks the congruity of default genesis data to
// corresponding hardcoded genesis hash values.
func TestGenesisHashes(t *testing.T) {
	for i, c := range []struct {
		genesis *genesisT.Genesis
		want    common.Hash
	}{
		{params.DefaultGenesisBlock(), params.MainnetGenesisHash},
		{params.DefaultGoerliGenesisBlock(), params.GoerliGenesisHash},
		{params.DefaultRinkebyGenesisBlock(), params.RinkebyGenesisHash},
		{params.DefaultSepoliaGenesisBlock(), params.SepoliaGenesisHash},
	} {
		// Test via MustCommit
		if have := MustCommitGenesis(rawdb.NewMemoryDatabase(), c.genesis).Hash(); have != c.want {
			t.Errorf("case: %d a), want: %s, got: %s", i, c.want.Hex(), have.Hex())
		}
	}
}

func TestGenesis_Commit(t *testing.T) {
	genesis := &genesisT.Genesis{
		BaseFee: big.NewInt(vars.InitialBaseFee),
		Config:  params.TestChainConfig,
		// difficulty is nil
	}

	db := rawdb.NewMemoryDatabase()
	genesisBlock := MustCommitGenesis(db, genesis)

	if genesis.Difficulty != nil {
		t.Fatalf("assumption wrong")
	}

	// This value should have been set as default in the ToBlock method.
	if genesisBlock.Difficulty().Cmp(vars.GenesisDifficulty) != 0 {
		t.Errorf("assumption wrong: want: %d, got: %v", vars.GenesisDifficulty, genesisBlock.Difficulty())
	}

	// Expect the stored total difficulty to be the difficulty of the genesis block.
	stored := rawdb.ReadTd(db, genesisBlock.Hash(), genesisBlock.NumberU64())

	if stored.Cmp(genesisBlock.Difficulty()) != 0 {
		t.Errorf("inequal difficulty; stored: %v, genesisBlock: %v", stored, genesisBlock.Difficulty())
	}
}

func TestReadWriteGenesisAlloc(t *testing.T) {
	var (
		db    = rawdb.NewMemoryDatabase()
		alloc = &genesisT.GenesisAlloc{
			{1}: {Balance: big.NewInt(1), Storage: map[common.Hash]common.Hash{{1}: {1}}},
			{2}: {Balance: big.NewInt(2), Storage: map[common.Hash]common.Hash{{2}: {2}}},
		}
		hash, _ = gaDeriveHash(alloc)
	)
	blob, _ := json.Marshal(alloc)
	rawdb.WriteGenesisStateSpec(db, hash, blob)

	var reload genesisT.GenesisAlloc
	err := reload.UnmarshalJSON(rawdb.ReadGenesisStateSpec(db, hash))
	if err != nil {
		t.Fatalf("Failed to load genesis state %v", err)
	}
	if len(reload) != len(*alloc) {
		t.Fatal("Unexpected genesis allocation")
	}
	for addr, account := range reload {
		want, ok := (*alloc)[addr]
		if !ok {
			t.Fatal("Account is not found")
		}
		if !reflect.DeepEqual(want, account) {
			t.Fatal("Unexpected account")
		}
	}
}
