# PHASE 1.4: Database Architecture Analysis
## Revised for ARBITRAGE_TRANS-First Trading Robot

**Date**: December 12, 2025  
**Status**: ✅ Complete Analysis  
**Target**: MySQL 9.4.0  

---

## 📊 Executive Summary

The system is an **arbitrage-driven crypto trading robot** where:
1. **PRIMARY Function**: Execute arbitrage transactions across exchanges (ARBITRAGE_TRANS)
2. **SECONDARY Functions**: Grid trading, other strategies (TRADE with TYPE reference)

Database structure now reflects this priority with proper relationships and configuration support for both markets (SPOT + FUTURES).

---

## 🏗️ Current Database Architecture (27 Tables)

### Core Trading Tables

#### 1. **ARBITRAGE_TRANS** (79 records)
**Purpose**: Primary transaction log for all arbitrage operations  
**Primary Key**: `ID` (int)  
**Key Relationships**:
- `TRADE_ID` → TRADE (which strategy executed this arbitrage)
- `STATUS` → ARBITRAGE_TRANS_STATUS (8 states)

**Structure**:
```sql
ID              INT PRIMARY KEY
TRADE_ID        INT FK → TRADE
STATUS          INT FK → ARBITRAGE_TRANS_STATUS
AMOUNT          DECIMAL(30,12)      -- Transaction amount
CALC_PRFIT      DECIMAL(30,12)      -- Calculated profit
DATE_CREATE     TIMESTAMP
DATE_MODIFY     TIMESTAMP
```

**Status States** (ARBITRAGE_TRANS_STATUS):
1. New - Record created, preparing to trade
2. In Progress - Tasks sent to workers
3. **Suspend** - Daemon crashed (most records here: 79 in state 3)
4. Error - Exchange transaction failed
5. Complete - Successful completion
6. Complete Loss - Completed with loss
7. Error Approved - Manual approval of error
8. Complete Loss Approved - Manual approval of loss

**Data Notes**:
- 79 transactions, all with `TRADE_ID = 7`
- All in status 3 (Suspend) → system halt on error
- AMOUNT and CALC_PRFIT are NULL (calculation happens during execution)

---

#### 2. **TRADE** (8 records)
**Purpose**: Trading strategy configurations  
**Primary Key**: `ID` (int AUTO_INCREMENT)  
**Key Relationships**:
- `UID` → USER
- `TYPE` → TRADE_TYPE

**Structure**:
```sql
ID                              INT PRIMARY KEY
UID                             INT FK → USER
TYPE                            INT FK → TRADE_TYPE
ACTIVE                          TINYINT DEFAULT 1
DESCRIPTION                     TEXT
DATE_CREATE                     TIMESTAMP
DATE_MODIFY                     TIMESTAMP
MAX_AMOUNT_TRADE                DECIMAL(30,12)      -- per-trade limit
-- Phase 1.4 additions (already in DB):
MAX_OPEN_ORDERS                 INT DEFAULT 10
MAX_POSITION_SIZE               DECIMAL(30,12)
STRATEGY_UPDATE_INTERVAL_SEC    INT DEFAULT 300
SLIPPAGE_PERCENT                DECIMAL(10,6) DEFAULT 0.1
ENABLE_BACKTEST                 TINYINT(1) DEFAULT 0
FIN_PROTECTION                  TINYINT(1) DEFAULT 0      -- New (safety limit)
BBO_ONLY                        TINYINT(1) DEFAULT 1      -- New (best bid-offer)
```

**Data**:
- 8 configs for UID 2 (one user - the developer)
- TYPE varies: types 1, 2, 5, 6 (mapped in TRADE_TYPE table)

**Purpose of TYPE field**:
- `TYPE=1`: Manual/test trading
- `TYPE=2`: Grid trading strategy
- `TYPE=5`: Market making
- `TYPE=6`: Arbitrage (primary)

---

#### 3. **TRADE_PAIR** (~1.3M records)
**Purpose**: Unified SPOT+FUTURES trading pairs catalog  
**Primary Key**: `ID` (int)  
**Unique Constraint**: `(MARKET_TYPE, BASE_CURRENCY_ID, QUOTE_CURRENCY_ID, EXCHANGE_ID)`

**Structure**:
```sql
ID                      INT PRIMARY KEY
MARKET_TYPE             ENUM('SPOT','FUTURES')  -- Market discriminator
BASE_CURRENCY_ID        INT FK → COIN
QUOTE_CURRENCY_ID       INT FK → COIN
EXCHANGE_ID             INT FK → EXCHANGE
ACTIVE                  TINYINT(1) DEFAULT 1
-- Pair identifiers:
BASE_VOLUME             DECIMAL(30,12)
QUOTE_VOLUME            DECIMAL(30,12)
MIN_PRICE_PRECISION     DECIMAL(10,6)
MIN_AMOUNT_PRECISION    DECIMAL(10,6)
-- Futures-specific (Phase 1.3):
LEVERAGE                DECIMAL(10,2)          -- Max leverage
FUNDING_RATE            DECIMAL(20,12)         -- Current funding %
CONTRACT_TYPE           VARCHAR(50)             -- PERPETUAL/QUARTERLY
-- Timestamps:
DATE_CREATE             TIMESTAMP
DATE_MODIFY             TIMESTAMP
```

**Data Distribution**:
- Market Type: All currently are 'SPOT' (1.3M+)
- Exchanges: Primarily exchange ID 825 (Binance equivalent)
- Status: Mix of ACTIVE=1 (trading) and ACTIVE=0 (disabled)
- NULL leverage/funding_rate (SPOT pairs don't use these)

**Critical Relationships**:
- Referenced by TRADE_PAIRS (trading config pairs)
- Referenced by MONITORING_TRADE_PAIRS (monitoring config pairs)
- Referenced by TRADE_HISTORY (execution history)
- Referenced by POS_POSITIONS (open positions)

---

### Configuration Linking Tables

#### 4. **TRADE_PAIRS** (Junction)
**Purpose**: Links TRADE configs to TRADE_PAIR entries + exchange account mapping  
**Primary Key**: Composite `(TRADE_ID, PAIR_ID, EAID)`

**Structure**:
```sql
TRADE_ID        INT FK → TRADE
PAIR_ID         INT FK → TRADE_PAIR
EAID            INT FK → EXCHANGE_ACCOUNTS    -- Which exchange account to use
PRIMARY KEY (TRADE_ID, PAIR_ID, EAID)
```

**Purpose**: One TRADE strategy can trade multiple pairs across multiple exchange accounts.

---

#### 5. **MONITORING_TRADE_PAIRS** (Junction)
**Purpose**: Links MONITORING configs to pairs being watched  
**Primary Key**: Composite `(MONITOR_ID, PAIR_ID)`

**Structure**:
```sql
MONITOR_ID      INT FK → MONITORING
PAIR_ID         INT FK → TRADE_PAIR
PRIMARY KEY (MONITOR_ID, PAIR_ID)
```

---

### Configuration Tables

#### 6. **MONITORING** (6 active + 1 inactive = 7 records)
**Purpose**: Orderbook/market data monitoring configurations  
**Primary Key**: `ID` (int AUTO_INCREMENT)  
**Key Relationship**: `UID` → USER

**Structure** (already complete in DB):
```sql
ID                      INT PRIMARY KEY
UID                     INT FK → USER
ORDERBOOK_DEPTH         INT DEFAULT 50       -- Depth to monitor
BATCH_SIZE              INT DEFAULT 1000     -- Records per batch
BATCH_INTERVAL_SEC      INT DEFAULT 300      -- Send every N seconds
RING_BUFFER_SIZE        INT DEFAULT 10000    -- Memory buffer
SAVE_INTERVAL_SEC       INT DEFAULT 600      -- DB save frequency
ACTIVE                  TINYINT(1) DEFAULT 1
DESCRIPTION             TEXT
DATE_CREATE             TIMESTAMP
DATE_MODIFY             TIMESTAMP
```

**Data** (6 active):
- Most for user UID=2
- DESCRIPTION shows purpose: 'LUNC', 'TON', 'erg/btc/usdt mon', etc.
- All use default batch config (50, 1000, 300, 10000, 600)

---

### Execution & State Tables

#### 7. **TRADE_HISTORY** (NEW - Phase 1.4)
**Purpose**: Complete trade execution history (SPOT + FUTURES unified)  
**Primary Key**: `ID` (bigint AUTO_INCREMENT)

**Structure**:
```sql
ID                  BIGINT PRIMARY KEY        -- Scale to billions
TRADE_ID            INT FK → TRADE            -- Which strategy
ORDER_ID            VARCHAR(128)              -- Exchange order ID
PAIR_ID             INT FK → TRADE_PAIR       -- Which pair (SPOT/FUTURES)
EAID                INT FK → EXCHANGE_ACCOUNTS
SIDE                ENUM('BUY','SELL')
PRICE               DECIMAL(30,12)
QUANTITY            DECIMAL(30,12)
COMMISSION          DECIMAL(30,12) DEFAULT 0
COMMISSION_ASSET    VARCHAR(16)               -- BNB, ETH, etc.
EXECUTED_TIME       BIGINT                    -- Microseconds Unix
STATUS              ENUM('PENDING','FILLED','PARTIAL','CANCELLED')
PROFIT_LOSS         DECIMAL(30,12)            -- P&L calculation
DATE_CREATE         TIMESTAMP
DATE_MODIFY         TIMESTAMP

INDEXES:
- PRIMARY KEY (ID)
- idx_th_trade_id (TRADE_ID)
- idx_th_order_id (ORDER_ID)
- idx_th_pair_id (PAIR_ID)
- idx_th_time (EXECUTED_TIME)
- idx_th_status (STATUS)
- idx_th_composite (TRADE_ID, EXECUTED_TIME)
```

---

#### 8. **DAEMON_STATE** (NEW - Phase 1.4)
**Purpose**: Daemon process lifecycle and state management  
**Primary Key**: `ID` (int)  
**Unique Constraint**: `DAEMON_NAME`

**Structure**:
```sql
ID                      INT PRIMARY KEY
DAEMON_NAME             VARCHAR(128) UNIQUE   -- hostname-pid
STATUS                  ENUM('STARTING','RUNNING','STOPPING','STOPPED','ERROR')
ROLE                    ENUM('MONITOR','TRADER','BOTH')
LAST_HEARTBEAT          BIGINT                -- Microseconds Unix
ACTIVE_MONITORING_ID    INT FK → MONITORING   -- Current config
ACTIVE_TRADE_ID         INT FK → TRADE        -- Current config
ERROR_MESSAGE           TEXT                  -- If STATUS=ERROR
DATE_CREATE             TIMESTAMP
DATE_MODIFY             TIMESTAMP

INDEXES:
- PRIMARY KEY (ID)
- UNIQUE idx_daemon_name (DAEMON_NAME)
- idx_daemon_status (STATUS)
- idx_daemon_heartbeat (LAST_HEARTBEAT)
```

**Use Cases**:
- Dead daemon detection (check LAST_HEARTBEAT > 5 minutes)
- Graceful shutdown (set STATUS=STOPPING)
- Error recovery (inspect ERROR_MESSAGE)

---

### Reference/Catalog Tables

#### 9. **TRADE_TYPE** (6 types)
```
ID  | NAME
----|-------------------
1   | Manual/Test
2   | Grid Trading
3   | (unused)
5   | Market Making
6   | Arbitrage
...
```

---

#### 10. **ARBITRAGE_TRANS_STATUS** (8 states)
Shown above in ARBITRAGE_TRANS section.

---

#### 11. **EXCHANGE** (multiple)
**Purpose**: Supported exchanges catalog  
**Key IDs in use**:
- 825: Binance equivalent
- 1, 2, 3, 3408, 1027, 2087, 19891: Other exchanges

---

#### 12. **EXCHANGE_ACCOUNTS** (EAID)
**Purpose**: User API keys for each exchange  
**Relationship**: User → EXCHANGE → Account credentials  
**Used by**: TRADE_PAIRS (which account executes which trades)

---

#### 13. **USER** (UID)
**Purpose**: System users  
**Primary user**: UID=2 (developer/tester)

---

#### 14. **COIN** (330+ cryptocurrencies)
**Purpose**: Crypto coin catalog  
**Key Fields**: ID, NAME, SYMBOL, SLUG  
**Examples**: BTC, ETH, BNB, USDT (stablecoins), etc.

---

#### 15. **CHAIN** (330+ blockchains)
**Purpose**: Blockchain network catalog  
**Examples**: ERC20, BEP20, TRC20, SOL, BTC, etc.  
**Used for**: Determining which chain a coin operates on

---

#### 16-19. **Position & Transaction Tables**

- **POS_POSITIONS**: Open positions (SPOT/FUTURES discriminator already present)
- **POS_TRANSACTIONS**: Atomic position transactions
- **PRICE_SPOT_LOG**: Historical spot prices
- **DEPOSIT/WITHDRAWAL**: Fund movement history

---

#### 20. **VERSION_DB**
Schema version tracking (for migrations).

---

#### 21-27. **User/Security Tables**
- **GROUP**: User groups/roles
- **USERS_GROUP**: User-to-group mapping
- **COIN_EXCEPTION**: Coins with special handling
- **UPDATE_STATUS**: (unused/archive table)

---

## 🔄 Data Flow: Arbitrage Robot Workflow

```
┌─────────────────────────────────────────────────────────────┐
│ MONITORING: Watch orderbook on multiple exchanges          │
│ - MONITORING config defines batch/buffer settings           │
│ - MONITORING_TRADE_PAIRS specifies which pairs to watch     │
│ - Reads from: TRADE_PAIR table (exchange + pair data)       │
│ - Outputs: Market data → Price feed / Alert system          │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ ARBITRAGE DETECTION: Find price discrepancies              │
│ - Buy on exchange A at price X                              │
│ - Sell on exchange B at price Y                             │
│ - Profit if (Y - X) > fees                                  │
│ - Route execution through TRADE config (TYPE=6)             │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ ARBITRAGE_TRANS: Create transaction record                 │
│ - STATUS: 'New' (record created)                            │
│ - TRADE_ID: Which strategy (TRADE config #)                │
│ - AMOUNT: How much to execute                               │
│ - CALC_PRFIT: Expected profit estimate                      │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ TRADE EXECUTION: Execute orders on both exchanges          │
│ - Find EXCHANGE_ACCOUNTS from TRADE_PAIRS mapping           │
│ - Execute BUY on exchange A                                 │
│ - Execute SELL on exchange B                                │
│ - Send task to workers                                      │
│ - ARBITRAGE_TRANS.STATUS → 'In Progress'                    │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ TRADE_HISTORY: Log all fills                               │
│ - One entry per order (BUY + SELL = 2 entries)             │
│ - TRADE_ID: Strategy that executed                          │
│ - PAIR_ID: Can be SPOT or FUTURES                           │
│ - PRICE, QUANTITY: Actual execution details                 │
│ - EXECUTED_TIME: Microsecond precision                      │
│ - STATUS: Track partial fills / cancellations               │
│ - PROFIT_LOSS: Realized P&L                                 │
└──────────────────────────┬──────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────────┐
│ ARBITRAGE_TRANS UPDATE: Mark completion                    │
│ - STATUS: 'Complete' (success) or 'Error' (failed)         │
│ - AMOUNT: Updated with actual amount traded                 │
│ - CALC_PRFIT: Updated with realized profit                  │
│ - DATE_MODIFY: Timestamp of completion                      │
└──────────────────────────────────────────────────────────────┘

If daemon crashes:
│ ARBITRAGE_TRANS.STATUS → 'Suspend'
│ DAEMON_STATE.STATUS → 'ERROR'
│ DAEMON_STATE.ERROR_MESSAGE → reason
└─ Recovery on daemon restart: find 'Suspend' records
```

---

## 🎯 Key Relationships Map

### Entry Point: ARBITRAGE_TRANS (Primary Flow)
```
ARBITRAGE_TRANS
    ├─ TRADE_ID ──────────────────────────→ TRADE (which strategy)
    │                                           ├─ UID ──────→ USER
    │                                           ├─ TYPE ─────→ TRADE_TYPE
    │                                           └─ refs TRADE_PAIRS junction
    │                                               ├─ PAIR_ID ──→ TRADE_PAIR
    │                                               └─ EAID ─────→ EXCHANGE_ACCOUNTS
    │
    └─ STATUS ─────────────────────────────→ ARBITRAGE_TRANS_STATUS
```

### Secondary: TRADE_HISTORY (Execution Logging)
```
TRADE_HISTORY
    ├─ TRADE_ID ──→ TRADE (same as above)
    ├─ PAIR_ID ───→ TRADE_PAIR (SPOT or FUTURES?)
    ├─ EAID ──────→ EXCHANGE_ACCOUNTS
    └─ EXECUTED_TIME → indexing for fast queries
```

### Configuration: MONITORING (Market Data)
```
MONITORING ──→ MONITORING_TRADE_PAIRS ──→ TRADE_PAIR
    ├─ UID ────→ USER
    └─ Batch/buffer config for data collection
```

### Daemon Health: DAEMON_STATE
```
DAEMON_STATE
    ├─ ACTIVE_MONITORING_ID ──→ MONITORING
    ├─ ACTIVE_TRADE_ID ───────→ TRADE
    └─ LAST_HEARTBEAT, STATUS for recovery
```

---

## 📈 Data Statistics

| Table | Records | Purpose |
|-------|---------|---------|
| ARBITRAGE_TRANS | 79 | Primary transactions (all in Suspend) |
| TRADE | 8 | Strategy configs |
| TRADE_PAIR | 1.3M+ | Pair catalog (SPOT) |
| MONITORING | 7 | Monitoring configs |
| USER | ~2 | System users (mainly UID=2) |
| EXCHANGE | 10+ | Exchange catalog |
| EXCHANGE_ACCOUNTS | ? | API key storage |
| COIN | 330+ | Crypto catalog |
| CHAIN | 330+ | Blockchain catalog |
| TRADE_HISTORY | 0 | Ready for execution logs |
| DAEMON_STATE | 0 | Ready for daemon tracking |

---

## 🚨 Critical Design Notes

### 1. ARBITRAGE_TRANS → TRADE Relationship
**Not a 1:1 mapping**. One TRADE config can execute multiple arbitrage transactions simultaneously:
- TRADE config defines strategy parameters (MAX_OPEN_ORDERS, SLIPPAGE_PERCENT, etc.)
- Multiple ARBITRAGE_TRANS records can link to same TRADE_ID
- Allows parallelization across multiple cryptocurrency pairs

### 2. Market Type Support (SPOT + FUTURES)
**TRADE_PAIR.MARKET_TYPE** ensures:
- SPOT pairs have NULL leverage/funding_rate
- FUTURES pairs have LEVERAGE, FUNDING_RATE, CONTRACT_TYPE populated
- Unique constraint `(MARKET_TYPE, BASE_ID, QUOTE_ID, EXCHANGE_ID)` allows same pair on same exchange in both markets

**Current State**: Only SPOT pairs in production (1.3M+), FUTURES ready for future expansion.

### 3. Timestamp Precision
- **TRADE_HISTORY.EXECUTED_TIME**: bigint (microseconds Unix)
  - Allows precise order matching between exchanges
  - Enables microsecond-resolution latency analysis
- **Other timestamps**: TIMESTAMP (MySQL standard)
  - Reduced precision acceptable for non-critical logging

### 4. Daemon Crash Recovery
**ARBITRAGE_TRANS.STATUS='Suspend'** indicates:
- Daemon crashed during execution
- Transaction is incomplete (may have partial fills)
- Manual inspection needed for recovery or replay
- 79 current records in this state → system needs recovery logic

### 5. Configuration Flexibility
Both TRADE and MONITORING support per-exchange configuration:
- **TRADE_PAIRS**: Different EAID (exchange account) for different pair+strategy combinations
- **MONITORING_TRADE_PAIRS**: Different monitoring configs for different pair subsets

---

## 🔧 Phase 1.4 Implementation Status

### ✅ Already in Database
1. ARBITRAGE_TRANS (primary table + status reference)
2. TRADE with all config columns:
   - MAX_OPEN_ORDERS, MAX_POSITION_SIZE
   - STRATEGY_UPDATE_INTERVAL_SEC, SLIPPAGE_PERCENT
   - ENABLE_BACKTEST, FIN_PROTECTION, BBO_ONLY
3. MONITORING with all config columns:
   - ORDERBOOK_DEPTH, BATCH_SIZE, BATCH_INTERVAL_SEC
   - RING_BUFFER_SIZE, SAVE_INTERVAL_SEC
4. TRADE_PAIR (renamed from SPOT_TRADE_PAIR) with market support:
   - MARKET_TYPE enum (currently all SPOT)
   - LEVERAGE, FUNDING_RATE, CONTRACT_TYPE (NULL for SPOT)

### ⏳ Pending Implementation
1. **TRADE_HISTORY**: Created (DDL present) but no records yet
2. **DAEMON_STATE**: Created (DDL present) but no records yet
3. **Go Code Integration**: Map these tables to types.go and business logic
4. **Recovery Logic**: Handle ARBITRAGE_TRANS with STATUS='Suspend'

---

## 📝 Phase 1.4 Deliverables

This analysis provides:
1. ✅ Complete table-by-table documentation
2. ✅ Data flow diagrams for arbitrage workflow
3. ✅ Key relationships map
4. ✅ Critical design notes for implementation
5. ✅ Status of Phase 1.4 tables (which exist, what's pending)
6. ✅ Timestamp precision specifications
7. ✅ Configuration architecture (SPOT/FUTURES ready)

---

## 🚀 Next Steps (Phase 1.5)

1. **Exchange Driver Interface**: Abstract away exchange API differences
2. **Arbitrage Detector**: Implement price diff scanning
3. **Order Executor**: Async order placement on multiple exchanges
4. **Crash Recovery**: Clean up 'Suspend' records, replay if needed
5. **Monitoring Integration**: Real-time orderbook updates from MONITORING configs
6. **Dashboard**: TRADE_HISTORY visualization + profit tracking

---

**Document Version**: 1.0  
**Last Updated**: 2025-12-12  
**Analyst**: System Architecture Analysis
