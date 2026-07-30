package api

import (
	"context"
	//nolint:gosec
	"crypto/md5" // G501: Blocklisted import crypto/md5: weak cryptographic primitive
	"encoding/hex"
	"encoding/json"
	"errors"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"

	clientnative "github.com/haproxytech/client-native/v6"
	"github.com/haproxytech/client-native/v6/config-parser/types"
	"github.com/haproxytech/client-native/v6/configuration"
	cfgoptions "github.com/haproxytech/client-native/v6/configuration/options"
	"github.com/haproxytech/client-native/v6/models"
	"github.com/haproxytech/client-native/v6/options"
	"github.com/haproxytech/client-native/v6/runtime"
	runtimeoptions "github.com/haproxytech/client-native/v6/runtime/options"

	"github.com/haproxytech/kubernetes-ingress/pkg/haproxy/instance"
	"github.com/haproxytech/kubernetes-ingress/pkg/store"
	"github.com/haproxytech/kubernetes-ingress/pkg/utils"
)

// BufferSize is the default value of HAproxy tune.bufsize. Not recommended to change it
// Map payload or socket data cannot be bigger than tune.bufsize
const BufferSize = 16000

// ErrNoActiveTransaction is returned when a commit is attempted with no active
// transaction. client-native resolves an empty transaction ID to the live
// configuration file (GetTransactionFile("") -> ConfigurationFile), and the
// success path of its commit deletes the transaction file it just committed --
// so committing with an empty ID deletes the live haproxy.cfg and leaves the
// process unable to reload. An empty ID is only ever legitimate for master
// parser reads, never for a commit.
var ErrNoActiveTransaction = errors.New("no active haproxy transaction: refusing to commit with an empty transaction id")

// ErrNotTransactionOwner is returned when a commit is attempted by a goroutine
// that did not start the active transaction. Without this check a stray commit
// would target whatever transaction another goroutine happens to have in
// flight, committing or deleting it out from under its owner.
var ErrNotTransactionOwner = errors.New("haproxy transaction is owned by another goroutine: refusing to commit it")

var logger = utils.GetLogger()

type HAProxyClient interface { //nolint:interfacebloat
	APIStartTransaction() error
	APICommitTransaction() error
	APIFinalCommitTransaction() error
	APIDisposeTransaction()
	ACL
	BackendsGet() models.Backends
	BackendGet(backendName string) (*models.Backend, error)
	// This function tests if a backend is existing :
	// Check if you're not rather looking for BackendUsed function.
	BackendExists(backendName string) bool
	// This function tests if a backend is existing AND IT'S USED.
	BackendUsed(backendName string) bool
	BackendCreatePermanently(backend models.Backend)
	BackendCreateIfNotExist(backend models.Backend)
	BackendCreateOrUpdate(backend models.Backend) (map[string][]interface{}, bool)
	BackendDelete(backendName string)
	BackendDeleteAllUnnecessary() ([]string, error)
	BackendCfgSnippetSet(backendName string, value []string) error
	BackendRuleDeleteAll(backend string)
	BackendServerDeleteAll(backendName string) error
	BackendServerCreate(backendName string, data models.Server) error
	BackendServerCreateOrUpdate(backendName string, data models.Server) error
	BackendServerEdit(backendName string, data models.Server) error
	BackendServerDelete(backendName string, serverName string) error
	BackendServerGet(serverName, backendNa string) (*models.Server, error)
	BackendServersGet(backendName string) (models.Servers, error)
	BackendSwitchingRule
	Capture
	DefaultsGetConfiguration() (*models.Defaults, error)
	DefaultsPushConfiguration(models.Defaults) error
	ExecuteRaw(command string) (result string, err error)
	Filter
	FrontendCfgSnippetSet(frontendName string, value []string) error
	FrontendCreate(frontend models.FrontendBase) error
	FrontendDelete(frontendName string) error
	FrontendsGet() (models.Frontends, error)
	FrontendGet(frontendName string) (models.Frontend, error)
	FrontendEdit(frontend models.FrontendBase) error
	FrontendEnableSSLOffload(frontendName string, certDir string, alpn string, strictSNI bool, generateCertificatesSigner string) (err error)
	FrontendDisableSSLOffload(frontendName string) (err error)
	FrontendSSLOffloadEnabled(frontendName string) bool
	Bind
	FrontendRuleDeleteAll(frontend string)
	GlobalGetLogTargets() (models.LogTargets, error)
	GlobalPushLogTargets(models.LogTargets) error
	GlobalGetConfiguration() (*models.Global, error)
	GlobalPushConfiguration(models.Global) error
	GlobalCfgSnippet(snippet []string) error
	GetMap(mapFile string) (*models.Map, error)
	HTTPRequestRule
	LogTarget
	TCPRequestRule
	PeerEntryDelete(peerSection string, name string) error
	PeerEntryEdit(peerSection string, peer models.PeerEntry) error
	PeerEntryCreateOrEdit(peerSection string, peer models.PeerEntry) error
	SetMapContent(mapFile string, payload []string) error
	SetServerAddrAndState([]RuntimeServerData) error
	SetAuxCfgFile(auxCfgFile string)
	SyncBackendSrvs(backend *store.RuntimeBackend, portUpdated bool) error
	UserListDeleteAll() error
	UserListExistsByGroup(group string) (bool, error)
	UserListCreateByGroup(group string, userPasswordMap map[string][]byte) error
	Cert
	CertAuth
	PushPreviousBackends() error
	PopPreviousBackends() error
}

type Cert interface {
	CertEntryCreate(filename string) error
	CertEntrySet(filename string, payload []byte) error
	CertEntryCommit(filename string) error
	CertEntryAbort(filename string) error
	CrtListEntryAdd(crtList string, entry runtime.CrtListEntry) error
	CrtListEntryDelete(crtList, filename string, linenumber *int64) error
	CertEntryDelete(filename string) error
}

type CertAuth interface {
	CertAuthEntryCreate(filename string) error
	CertAuthEntrySet(filename string, payload []byte) error
	CertAuthEntryCommit(filename string) error
	CertEntryAbort(filename string) error
	CertEntryDelete(filename string) error
}

type Backend struct { // use same names as in client native v6
	models.Backend
	ConfigSnippets []string
	Permanent      bool
	Used           bool
}

type ACL interface {
	ACLsGet(parentType, parentName string, aclName ...string) (models.Acls, error)
	ACLDeleteAll(parentType string, parentName string) error
	ACLCreate(id int64, parentType string, parentName string, data *models.ACL) error
	ACLsReplace(parentType, parentName string, rules models.Acls) error
}

type BackendSwitchingRule interface {
	BackendSwitchingRulesGet(frontendName string) (models.BackendSwitchingRules, error)
	BackendSwitchingRuleCreate(id int64, frontend string, rule models.BackendSwitchingRule) error
	BackendSwitchingRuleDeleteAll(frontend string) error
	BackendSwitchingRulesReplace(frontend string, rules models.BackendSwitchingRules) error
}

type Bind interface {
	FrontendBindsGet(frontend string) (models.Binds, error)
	FrontendBindCreate(frontend string, bind models.Bind) error
	FrontendBindEdit(frontend string, bind models.Bind) error
	FrontendBindDelete(frontend string, bind string) error
}

type Filter interface {
	FilterCreate(id int64, parentType, parentName string, rule models.Filter) error
	FiltersGet(parentType, parentName string) (models.Filters, error)
	FilterDeleteAll(parentType, parentName string) (err error)
	FiltersReplace(parentType, parentName string, rules models.Filters) error
}

type Capture interface {
	CaptureCreate(id int64, frontend string, rule models.Capture) error
	CaptureDeleteAll(frontend string) (err error)
	CapturesGet(frontend string) (models.Captures, error)
	CapturesReplace(frontend string, rules models.Captures) error
}

type LogTarget interface {
	LogTargetCreate(id int64, parentType, parentName string, rule models.LogTarget) error
	LogTargetsGet(parentType, parentName string) (models.LogTargets, error)
	LogTargetDeleteAll(parentType, parentName string) (err error)
	LogTargetsReplace(parentType, parentName string, rules models.LogTargets) error
}

type TCPRequestRule interface {
	TCPRequestRuleCreate(id int64, parentType, parentName string, rule models.TCPRequestRule) error
	TCPRequestRulesGet(parentType, parentName string) (models.TCPRequestRules, error)
	TCPRequestRuleDeleteAll(parentType, parentName string) (err error)
	TCPRequestRulesReplace(parentType, parentName string, rules models.TCPRequestRules) error
	FrontendTCPRequestRuleCreate(id int64, frontend string, rule models.TCPRequestRule, ingressACL string) error
}

type HTTPRequestRule interface {
	HTTPRequestRulesGet(parentType, parentName string) (models.HTTPRequestRules, error)
	HTTPRequestRuleDeleteAll(parentType string, parentName string) error
	HTTPRequestRuleCreate(id int64, parentType string, parentName string, data *models.HTTPRequestRule) error
	HTTPRequestRulesReplace(parentType, parentName string, rules models.HTTPRequestRules) error
	FrontendHTTPRequestRuleCreate(id int64, frontend string, rule models.HTTPRequestRule, ingressACL string) error
	FrontendHTTPResponseRuleCreate(id int64, frontend string, rule models.HTTPResponseRule, ingressACL string) error
	FrontendHTTPAfterResponseRuleCreate(id int64, frontend string, rule models.HTTPAfterResponseRule, ingressACL string) error
	BackendHTTPRequestRuleCreate(id int64, backend string, rule models.HTTPRequestRule) error
}

type clientNative struct {
	nativeAPI                           clientnative.HAProxyClient
	activeTransaction                   string
	backends                            map[string]Backend
	previousBackends                    []byte
	configurationHashAtTransactionStart string

	// txMu serializes the entire transaction lifecycle, not just the commit.
	// activeTransaction, configurationHashAtTransactionStart and the backends map
	// are shared mutable state, and one client is shared by every frontend
	// Manager: without this, one goroutine's dispose clears the transaction ID
	// another goroutine is still committing with, and a commit with an empty ID
	// deletes the live haproxy.cfg. Acquired by a successful APIStartTransaction,
	// released by APIDisposeTransaction.
	txMu sync.Mutex

	// txStateMu guards txOwner, the ID of the goroutine whose started
	// transaction currently holds txMu (0 = none). Tracking the owner — not just
	// a boolean — keeps APIDisposeTransaction safe against every stray-dispose
	// shape: disposing with no transaction started, disposing twice, and the
	// nastier interleaving where goroutine A disposes, goroutine B starts, and
	// A's *deferred* dispose then fires — with a plain boolean that deferred
	// dispose would clear B's transaction ID and unlock B's mutex mid-commit.
	// Consequence: a transaction must be disposed by the goroutine that
	// started it, which every caller (defer in the starting function) satisfies.
	txStateMu sync.Mutex
	txOwner   uint64
}

// goroutineID returns the current goroutine's ID by parsing the stack header.
// There is no public API for this; the parse is the standard trick and costs
// far less than the file I/O and exec a transaction already performs.
func goroutineID() uint64 {
	var buf [64]byte
	n := goruntime.Stack(buf[:], false)
	s := strings.TrimPrefix(string(buf[:n]), "goroutine ")
	if i := strings.IndexByte(s, ' '); i > 0 {
		if id, err := strconv.ParseUint(s[:i], 10, 64); err == nil {
			return id
		}
	}
	return 0
}

func New(transactionDir, configFile, programPath, runtimeSocket string) (client HAProxyClient, err error) { //nolint:ireturn
	var runtimeClient runtime.Runtime
	if runtimeSocket != "" {
		runtimeClient, err = runtime.New(context.Background(), runtimeoptions.Socket(runtimeSocket), runtimeoptions.DoNotCheckRuntimeOnInit)
	} else {
		runtimeClient, err = runtime.New(context.Background())
	}
	if err != nil {
		return nil, err
	}

	confClient, err := configuration.New(context.Background(),
		cfgoptions.ConfigurationFile(configFile),
		cfgoptions.HAProxyBin(programPath),
		cfgoptions.UseModelsValidation,
		cfgoptions.UseMd5Hash,
		cfgoptions.TransactionsDir(transactionDir),
	)
	if err != nil {
		return nil, err
	}

	opt := []options.Option{
		options.Configuration(confClient),
		options.Runtime(runtimeClient),
	}
	cnHAProxyClient, err := clientnative.New(context.Background(), opt...)
	if err != nil {
		return nil, err
	}

	cn := clientNative{
		nativeAPI: cnHAProxyClient,
		backends:  make(map[string]Backend),
	}
	return &cn, nil
}

// APIStartTransaction opens a configuration transaction and takes ownership of
// the client until APIDisposeTransaction releases it. Callers must dispose every
// transaction they successfully start, conventionally with a defer, and from
// the same goroutine that started it.
func (c *clientNative) APIStartTransaction() error {
	c.txMu.Lock()
	c.setTxOwner(goroutineID())

	if err := c.startTransaction(); err != nil {
		// A failed start owns nothing: drop any half-set ID so a later stray
		// commit is caught by the empty-ID guard rather than committing an
		// orphaned transaction, and release the lock, because callers return the
		// error without disposing.
		c.activeTransaction = ""
		c.setTxOwner(0)
		c.txMu.Unlock()
		return err
	}
	return nil
}

// startTransaction does the actual work of APIStartTransaction. It runs with
// txMu held.
func (c *clientNative) startTransaction() error {
	configuration, err := c.nativeAPI.Configuration()
	if err != nil {
		return err
	}
	version, errVersion := configuration.GetVersion("")
	if errVersion != nil || version < 1 {
		// silently fallback to 1
		version = 1
	}
	transaction, err := configuration.StartTransaction(version)
	if err != nil {
		return err
	}
	logger.WithField(utils.LogFieldTransactionID, transaction.ID)
	c.activeTransaction = transaction.ID

	hash, err := c.computeConfigurationHash(configuration)
	if err != nil {
		return err
	}
	c.configurationHashAtTransactionStart = hash

	return nil
}

func (c *clientNative) computeConfigurationHash(configuration configuration.Configuration) (string, error) {
	p, err := configuration.GetParser(c.activeTransaction)
	if err != nil {
		return "", err
	}
	// Note that p.String() does not include the hash!!!
	content := p.String()
	//nolint: gosec
	hash := md5.Sum([]byte(content))
	return hex.EncodeToString(hash[:]), err
}

func (c *clientNative) APICommitTransaction() error {
	if c.activeTransaction == "" {
		return ErrNoActiveTransaction
	}
	if !c.isTxOwner() {
		return ErrNotTransactionOwner
	}

	configuration, err := c.nativeAPI.Configuration()
	if err != nil {
		return err
	}

	hash, err := c.computeConfigurationHash(configuration)
	if err != nil {
		return err
	}

	if c.configurationHashAtTransactionStart == hash {
		if errDel := configuration.DeleteTransaction(c.activeTransaction); errDel != nil {
			return errDel
		}
		return nil
	}
	_, err = configuration.CommitTransaction(c.activeTransaction)
	return err
}

func (c *clientNative) APIFinalCommitTransaction() error {
	// The guards have to come before BackendDeleteAllUnnecessary and the backend
	// processing loop below: with an empty transaction ID those would mutate the
	// live configuration through the master parser instead of a transaction, and
	// a non-owner would process them into another goroutine's transaction.
	if c.activeTransaction == "" {
		return ErrNoActiveTransaction
	}
	if !c.isTxOwner() {
		return ErrNotTransactionOwner
	}

	configuration, err := c.nativeAPI.Configuration()
	if err != nil {
		return err
	}

	var errs utils.Errors
	// First we remove all backends ...
	deletedBackends, _ := c.BackendDeleteAllUnnecessary()
	for _, deletedBackend := range deletedBackends {
		instance.Reload("backend '%s' deleted", deletedBackend)
	}
	// ... then we parse the backends to take decisions.
	for backendName, backend := range c.backends {
		errs.Add(c.processBackend(&backend.Backend, configuration))
		errs.AddErrors(c.processServers(backendName, configuration))
		errs.Add(c.processConfigSnippets(backendName, backend.ConfigSnippets, configuration))
		errs.AddErrors(c.processACLs(backendName, backend.ACLList, configuration))
		errs.AddErrors(c.processHTTPRequestRules(backendName, backend.HTTPRequestRuleList, configuration))
		backend.Used = false
		c.backends[backendName] = backend
	}

	hash, err := c.computeConfigurationHash(configuration)
	if err != nil {
		return err
	}

	if c.configurationHashAtTransactionStart == hash {
		if errDel := configuration.DeleteTransaction(c.activeTransaction); errDel != nil {
			errs.Add(errDel)
		}
		return errs.Result()
	}
	_, err = configuration.CommitTransaction(c.activeTransaction)
	logger.Error(errs.Result())
	return err
}

// TransactionLocker is the transaction-lifecycle lock of a client, exposed so
// code that must observe a quiescent client — no transaction mid-flight — can
// hold it without opening a transaction. The reload monitor is the intended
// user: checking the reload flag and reloading the on-disk config while a
// final commit is between flag-set and file-write would reload the old file
// and then clear the flag, silently dropping the newer configuration.
// Callers must not start a transaction while holding the lock.
type TransactionLocker interface {
	TxLock()
	TxUnlock()
}

// TxLock acquires the transaction-lifecycle lock without opening a
// transaction. See TransactionLocker.
func (c *clientNative) TxLock() { c.txMu.Lock() }

// TxUnlock releases the lock taken by TxLock.
func (c *clientNative) TxUnlock() { c.txMu.Unlock() }

// APIDisposeTransaction ends the transaction opened by APIStartTransaction and
// releases the client. It is a no-op unless the calling goroutine owns the
// active transaction, so a dispose after a failed start, a double dispose, or
// a deferred dispose firing after another goroutine has since started its own
// transaction never clears state or releases a lock it does not own. The
// logger fields are likewise only reset when a transaction is actually
// disposed — an unconditional reset would strip the transactionID field from
// a transaction in flight on another goroutine.
func (c *clientNative) APIDisposeTransaction() {
	if !c.takeTxOwnership() {
		return
	}
	logger.ResetFields()
	c.activeTransaction = ""
	c.txMu.Unlock()
}

func (c *clientNative) setTxOwner(gid uint64) {
	c.txStateMu.Lock()
	c.txOwner = gid
	c.txStateMu.Unlock()
}

// isTxOwner reports whether the calling goroutine owns the active transaction.
func (c *clientNative) isTxOwner() bool {
	gid := goroutineID()
	c.txStateMu.Lock()
	defer c.txStateMu.Unlock()
	return c.txOwner != 0 && c.txOwner == gid
}

// takeTxOwnership clears ownership and reports whether the calling goroutine
// was the owner — the one responsible for releasing txMu.
func (c *clientNative) takeTxOwnership() bool {
	gid := goroutineID()
	c.txStateMu.Lock()
	defer c.txStateMu.Unlock()
	if c.txOwner == 0 || c.txOwner != gid {
		return false
	}
	c.txOwner = 0
	return true
}

func (c *clientNative) SetAuxCfgFile(auxCfgFile string) {
	configuration, err := c.nativeAPI.Configuration()
	if err != nil {
		logger := logger
		logger.Error(err)
	}
	if auxCfgFile == "" {
		configuration.SetValidateConfigFiles(nil, nil)
		return
	}
	configuration.SetValidateConfigFiles(nil, []string{auxCfgFile})
}

func (c *clientNative) processBackend(backend *models.Backend, configuration configuration.Configuration) error {
	// Try to create the backend ...
	errCreateBackend := configuration.CreateBackend(backend, c.activeTransaction, 0)
	if errCreateBackend != nil {
		// ... maybe it's already existing, so just edit it.
		return configuration.EditBackend(backend.Name, backend, c.activeTransaction, 0)
	}
	return nil
}

func (c *clientNative) processServers(backendName string, configuration configuration.Configuration) utils.Errors {
	var errs utils.Errors
	// Same for servers.
	servers, _ := c.BackendServersGet(backendName)
	for _, server := range servers {
		errCreateServer := configuration.CreateServer("backend", backendName, server, c.activeTransaction, 0)
		if errCreateServer != nil {
			errs.Add(configuration.EditServer(server.Name, "backend", backendName, server, c.activeTransaction, 0))
		} else {
			// Server has been created, a reload is required
			// It covers the case where there was a failure, scaleHAProxySrvs has already been called in a previous loop
			// but the sync failed (wrong config)
			// When the config is fixed, servers will be created
			instance.Reload("server '%s' created in backend '%s'", server.Name, backendName)
		}
	}
	return errs
}

func (c *clientNative) processConfigSnippets(backendName string, configSnippets []string, configuration configuration.Configuration) error {
	// Same for backend configsnippets.
	config, err := configuration.GetParser(c.activeTransaction)
	if err != nil {
		return err
	}
	if len(configSnippets) > 0 {
		return config.Set("backend", backendName, "config-snippet", types.StringSliceC{Value: configSnippets})
	} else {
		return config.Set("backend", backendName, "config-snippet", nil)
	}
}

func (c *clientNative) processACLs(backendName string, aclsList models.Acls, configuration configuration.Configuration) utils.Errors {
	// we remove all acls because of permanent backend still in parsers.
	_, existingACLs, _ := configuration.GetACLs("backend", backendName, c.activeTransaction)
	for range existingACLs {
		_ = configuration.DeleteACL(0, "backend", backendName, c.activeTransaction, 0)
	}
	var errs utils.Errors
	// we (re)create all acls
	for _, acl := range aclsList {
		errs.Add(configuration.CreateACL(0, "backend", backendName, acl, c.activeTransaction, 0))
	}
	return errs
}

func (c *clientNative) processHTTPRequestRules(backendName string, httpRequestsRules models.HTTPRequestRules, configuration configuration.Configuration) utils.Errors {
	// we remove all http request rules because of permanent backend still in parsers.
	_, existingHTTPRequestRules, _ := configuration.GetHTTPRequestRules("backend", backendName, c.activeTransaction)
	for range existingHTTPRequestRules {
		_ = configuration.DeleteHTTPRequestRule(0, "backend", backendName, c.activeTransaction, 0)
	}
	var errs utils.Errors
	// we (re)create all http request rules
	for _, httpRequestRule := range httpRequestsRules {
		errs.Add(configuration.CreateHTTPRequestRule(0, "backend", backendName, httpRequestRule, c.activeTransaction, 0))
	}
	return errs
}

func (c *clientNative) PushPreviousBackends() error {
	logger.Debug("Pushing backends as previous successfully applied backends")
	jsonBackends, err := json.Marshal(c.backends)
	if err != nil {
		return err
	}
	c.previousBackends = jsonBackends
	return nil
}

func (c *clientNative) PopPreviousBackends() error {
	logger.Debug("Popping backends from previous successfully applied backends")
	if c.previousBackends == nil {
		clear(c.backends)
		return nil
	}
	backends := map[string]Backend{}
	err := json.Unmarshal(c.previousBackends, &backends)
	if err != nil {
		return err
	}
	c.backends = backends
	return nil
}
