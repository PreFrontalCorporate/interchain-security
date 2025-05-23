package keeper

import (
	"crypto/rand"
	"encoding/binary"
	"testing"
	"time"

	dbm "github.com/cosmos/cosmos-db"
	clienttypes "github.com/cosmos/ibc-go/v10/modules/core/02-client/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log"
	math "cosmossdk.io/math"
	"cosmossdk.io/store"
	"cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/address"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	paramstypes "github.com/cosmos/cosmos-sdk/x/params/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	abci "github.com/cometbft/cometbft/abci/types"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"

	consumerkeeper "github.com/cosmos/interchain-security/v7/x/ccv/consumer/keeper"
	consumertypes "github.com/cosmos/interchain-security/v7/x/ccv/consumer/types"
	providerkeeper "github.com/cosmos/interchain-security/v7/x/ccv/provider/keeper"
	providertypes "github.com/cosmos/interchain-security/v7/x/ccv/provider/types"
	"github.com/cosmos/interchain-security/v7/x/ccv/types"
)

// Parameters needed to instantiate an in-memory keeper
type InMemKeeperParams struct {
	Cdc            *codec.ProtoCodec
	StoreKey       *storetypes.KVStoreKey
	ParamsSubspace *paramstypes.Subspace
	Ctx            sdk.Context
}

// NewInMemKeeperParams instantiates in-memory keeper params with default values
func NewInMemKeeperParams(tb testing.TB) InMemKeeperParams {
	tb.Helper()
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	memStoreKey := storetypes.NewMemoryStoreKey(types.MemStoreKey)

	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	stateStore.MountStoreWithDB(memStoreKey, storetypes.StoreTypeMemory, nil)
	require.NoError(tb, stateStore.LoadLatestVersion())

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry) // Public key implementation registered here
	cdc := codec.NewProtoCodec(registry)

	paramsSubspace := paramstypes.NewSubspace(cdc,
		codec.NewLegacyAmino(),
		storeKey,
		memStoreKey,
		paramstypes.ModuleName,
	)
	ctx := sdk.NewContext(stateStore, tmproto.Header{}, false, log.NewNopLogger())

	return InMemKeeperParams{
		Cdc:            cdc,
		StoreKey:       storeKey,
		ParamsSubspace: &paramsSubspace,
		Ctx:            ctx,
	}
}

// A struct holding pointers to any mocked external keeper needed for provider/consumer keeper setup.
type MockedKeepers struct {
	*MockChannelKeeper
	*MockConnectionKeeper
	*MockClientKeeper
	*MockStakingKeeper
	*MockSlashingKeeper
	*MockAccountKeeper
	*MockBankKeeper
	*MockIBCTransferKeeper
	*MockIBCCoreKeeper
	*MockDistributionKeeper
	// *MockGovKeeper
}

// NewMockedKeepers instantiates a struct with pointers to properly instantiated mocked keepers.
func NewMockedKeepers(ctrl *gomock.Controller) MockedKeepers {
	return MockedKeepers{
		MockChannelKeeper:      NewMockChannelKeeper(ctrl),
		MockConnectionKeeper:   NewMockConnectionKeeper(ctrl),
		MockClientKeeper:       NewMockClientKeeper(ctrl),
		MockStakingKeeper:      NewMockStakingKeeper(ctrl),
		MockSlashingKeeper:     NewMockSlashingKeeper(ctrl),
		MockAccountKeeper:      NewMockAccountKeeper(ctrl),
		MockBankKeeper:         NewMockBankKeeper(ctrl),
		MockIBCTransferKeeper:  NewMockIBCTransferKeeper(ctrl),
		MockIBCCoreKeeper:      NewMockIBCCoreKeeper(ctrl),
		MockDistributionKeeper: NewMockDistributionKeeper(ctrl),
	}
}

// NewInMemProviderKeeper instantiates an in-mem provider keeper from params and mocked keepers
func NewInMemProviderKeeper(params InMemKeeperParams, mocks MockedKeepers) providerkeeper.Keeper {
	return providerkeeper.NewKeeper(
		params.Cdc,
		params.StoreKey,
		*params.ParamsSubspace,
		mocks.MockChannelKeeper,
		mocks.MockConnectionKeeper,
		mocks.MockClientKeeper,
		mocks.MockStakingKeeper,
		mocks.MockSlashingKeeper,
		mocks.MockAccountKeeper,
		mocks.MockDistributionKeeper,
		mocks.MockBankKeeper,
		// mocks.MockGovKeeper,
		govkeeper.Keeper{}, // HACK: to make parts of the test work
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		address.NewBech32Codec("cosmosvaloper"),
		address.NewBech32Codec("cosmosvalcons"),
		authtypes.FeeCollectorName,
	)
}

// NewInMemConsumerKeeper instantiates an in-mem consumer keeper from params and mocked keepers
func NewInMemConsumerKeeper(params InMemKeeperParams, mocks MockedKeepers) consumerkeeper.Keeper {
	return consumerkeeper.NewKeeper(
		params.Cdc,
		params.StoreKey,
		mocks.MockChannelKeeper,
		mocks.MockConnectionKeeper,
		mocks.MockClientKeeper,
		mocks.MockSlashingKeeper,
		mocks.MockBankKeeper,
		mocks.MockAccountKeeper,
		mocks.MockIBCTransferKeeper,
		mocks.MockIBCCoreKeeper,
		authtypes.FeeCollectorName,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		address.NewBech32Codec("cosmosvaloper"),
		address.NewBech32Codec("cosmosvalcons"),
	)
}

// Returns an in-memory provider keeper, context, controller, and mocks, given a test instance and parameters.
//
// Note: Calling ctrl.Finish() at the end of a test function ensures that
// no unexpected calls to external keepers are made.
func GetProviderKeeperAndCtx(t *testing.T, params InMemKeeperParams) (
	providerkeeper.Keeper, sdk.Context, *gomock.Controller, MockedKeepers,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mocks := NewMockedKeepers(ctrl)
	return NewInMemProviderKeeper(params, mocks), params.Ctx, ctrl, mocks
}

// Return an in-memory consumer keeper, context, controller, and mocks, given a test instance and parameters.
//
// Note: Calling ctrl.Finish() at the end of a test function ensures that
// no unexpected calls to external keepers are made.
func GetConsumerKeeperAndCtx(t *testing.T, params InMemKeeperParams) (
	consumerkeeper.Keeper, sdk.Context, *gomock.Controller, MockedKeepers,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mocks := NewMockedKeepers(ctrl)
	return NewInMemConsumerKeeper(params, mocks), params.Ctx, ctrl, mocks
}

type PrivateKey struct {
	PrivKey cryptotypes.PrivKey
}

// Obtains slash packet data with a newly generated key, and randomized field values
func GetNewSlashPacketData() types.SlashPacketData {
	b1 := make([]byte, 8)
	_, _ = rand.Read(b1)
	b2 := make([]byte, 8)
	_, _ = rand.Read(b2)
	b3 := make([]byte, 8)
	_, _ = rand.Read(b3)
	return types.SlashPacketData{
		Validator: abci.Validator{
			Address: ed25519.GenPrivKey().PubKey().Address(),
			Power:   int64(binary.BigEndian.Uint64(b1)),
		},
		ValsetUpdateId: binary.BigEndian.Uint64(b2),
		Infraction:     stakingtypes.Infraction(binary.BigEndian.Uint64(b3) % 3),
	}
}

// SetupForDeleteConsumerChain registers expected mock calls and corresponding state setup
// which assert that a consumer chain was properly setup to be later deleted with `DeleteConsumerChain`.
// Note: This function only setups and tests that we correctly setup a consumer chain that we could later delete when
// calling `DeleteConsumerChain` -- this does NOT necessarily mean that the consumer chain is deleted.
// Also see `TestProviderStateIsCleanedAfterConsumerChainIsDeleted`.
func SetupForDeleteConsumerChain(t *testing.T, ctx sdk.Context,
	providerKeeper *providerkeeper.Keeper, mocks MockedKeepers,
	consumerId string,
) {
	t.Helper()

	expectations := GetMocksForCreateConsumerClient(ctx, &mocks,
		"chainID", clienttypes.NewHeight(0, 5))
	expectations = append(expectations, GetMocksForSetConsumerChain(ctx, &mocks, "chainID")...)

	gomock.InOrder(expectations...)

	providerKeeper.SetConsumerChainId(ctx, consumerId, "chainID")
	err := providerKeeper.SetConsumerMetadata(ctx, consumerId, GetTestConsumerMetadata())
	require.NoError(t, err)
	err = providerKeeper.SetConsumerInitializationParameters(ctx, consumerId, GetTestInitializationParameters())
	require.NoError(t, err)
	err = providerKeeper.SetConsumerPowerShapingParameters(ctx, consumerId, GetTestPowerShapingParameters())
	require.NoError(t, err)

	// set the chain to initialized so that we can create a consumer client
	providerKeeper.SetConsumerPhase(ctx, consumerId, providertypes.CONSUMER_PHASE_INITIALIZED)

	err = providerKeeper.CreateConsumerClient(ctx, consumerId, []byte{})
	require.NoError(t, err)
	// set the mapping consumer ID <> client ID for the consumer chain
	providerKeeper.SetConsumerClientId(ctx, consumerId, "clientID")
	// set the channel ID for the consumer chain
	err = providerKeeper.SetConsumerChain(ctx, "channelID")
	require.NoError(t, err)

	// set the chain to stopped sto the chain can be deleted
	providerKeeper.SetConsumerPhase(ctx, consumerId, providertypes.CONSUMER_PHASE_STOPPED)
}

// TestProviderStateIsCleanedAfterConsumerChainIsDeleted executes test assertions for the provider's state being cleaned
// after a deleted consumer chain.
func TestProviderStateIsCleanedAfterConsumerChainIsDeleted(t *testing.T, ctx sdk.Context, providerKeeper providerkeeper.Keeper,
	consumerId, expectedChannelID string, expErr bool,
) {
	t.Helper()
	_, found := providerKeeper.GetConsumerClientId(ctx, consumerId)
	require.False(t, found)
	_, found = providerKeeper.GetConsumerIdToChannelId(ctx, consumerId)
	require.False(t, found)
	_, found = providerKeeper.GetChannelIdToConsumerId(ctx, expectedChannelID)
	require.False(t, found)
	_, found = providerKeeper.GetInitChainHeight(ctx, consumerId)
	require.False(t, found)
	acks := providerKeeper.GetSlashAcks(ctx, consumerId)
	require.Empty(t, acks)

	// test key assignment state is cleaned
	require.Empty(t, providerKeeper.GetAllValidatorConsumerPubKeys(ctx, &consumerId))
	require.Empty(t, providerKeeper.GetAllValidatorsByConsumerAddr(ctx, &consumerId))
	require.Empty(t, providerKeeper.GetAllConsumerAddrsToPrune(ctx, consumerId))
	require.Empty(t, providerKeeper.GetAllCommissionRateValidators(ctx, consumerId))
	require.Zero(t, providerKeeper.GetEquivocationEvidenceMinHeight(ctx, consumerId))
}

func GetTestConsumerMetadata() providertypes.ConsumerMetadata {
	return providertypes.ConsumerMetadata{
		Name:        "chain name",
		Description: "description",
		Metadata:    "metadata",
	}
}

func GetTestInitializationParameters() providertypes.ConsumerInitializationParameters {
	return providertypes.ConsumerInitializationParameters{
		InitialHeight:                     clienttypes.NewHeight(0, 5),
		GenesisHash:                       []byte("gen_hash"),
		BinaryHash:                        []byte("bin_hash"),
		SpawnTime:                         time.Now().UTC(),
		ConsumerRedistributionFraction:    types.DefaultConsumerRedistributeFrac,
		BlocksPerDistributionTransmission: types.DefaultBlocksPerDistributionTransmission,
		DistributionTransmissionChannel:   "",
		HistoricalEntries:                 types.DefaultHistoricalEntries,
		CcvTimeoutPeriod:                  types.DefaultCCVTimeoutPeriod,
		TransferTimeoutPeriod:             types.DefaultTransferTimeoutPeriod,
		UnbondingPeriod:                   types.DefaultConsumerUnbondingPeriod,
	}
}

func GetTestInfractionParameters() providertypes.InfractionParameters {
	return providertypes.InfractionParameters{
		DoubleSign: &providertypes.SlashJailParameters{
			JailDuration:  1200 * time.Second,
			SlashFraction: math.LegacyNewDecWithPrec(5, 1), // 0.5
			Tombstone:     true,
		},
		Downtime: &providertypes.SlashJailParameters{
			JailDuration:  600 * time.Second,
			SlashFraction: math.LegacyNewDec(0),
			Tombstone:     false,
		},
	}
}

func GetTestPowerShapingParameters() providertypes.PowerShapingParameters {
	return providertypes.PowerShapingParameters{
		Top_N:              0,
		ValidatorsPowerCap: 0,
		ValidatorSetCap:    0,
		Allowlist:          nil,
		Denylist:           nil,
		MinStake:           0,
		AllowInactiveVals:  false,
		Prioritylist:       nil,
	}
}

// Obtains a CrossChainValidator with a newly generated key, and randomized field values
func GetNewCrossChainValidator(t *testing.T) consumertypes.CrossChainValidator {
	t.Helper()
	b1 := make([]byte, 8)
	_, _ = rand.Read(b1)
	power := int64(binary.BigEndian.Uint64(b1))
	privKey := ed25519.GenPrivKey()
	validator, err := consumertypes.NewCCValidator(privKey.PubKey().Address(), power, privKey.PubKey())
	require.NoError(t, err)
	return validator
}

// Must panics if err is not nil, otherwise returns v.
// This is useful to get a value from a function that returns a value and an error
// in a single line.
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
