package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/soden46/hyperlux-chain/consensus"
	"github.com/soden46/hyperlux-chain/ledger"
	"github.com/soden46/hyperlux-chain/network"
	"github.com/soden46/hyperlux-chain/wallet"
)

// RunCLI adalah fungsi utama untuk menjalankan antarmuka baris perintah
func RunCLI() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	// Muat semua data ke memori sekali di awal
	ledger.LoadAllData()

	cmd := os.Args[1]

	switch cmd {
	// ================= LEDGER & NODE =================
	case "init":
		handleInit()
	case "start":
		handleStart()

	// ================= TRANSAKSI =================
	case "tx":
		handleTx()
	case "tx-bulk":
		handleTxBulk()
	case "tx-bulk-random-parallel":
		handleTxBulkRandomParallel()
	case "tx-bulk-multi":
		handleTxBulkMulti()

	// ================= PENGUJIAN & OTOMASI =================
	case "stress-test":
		handleStressTest()
	case "auto-commit":
		handleAutoCommit()
	case "stress-and-commit":
		handleStressAndCommit()

	// ================= WALLET & TOOLS =================
	case "wallet-bulk":
		handleWalletBulk()
	case "metrics":
		handleMetrics()
	case "commit":
		handleCommit()
	case "airdrop":
		handleAirdrop()
	case "fix-validators":
		handleFixValidators()
	case "full-test":
		handleFullTest()

	// ================= VALIDATOR SECURITY (baru) =================
	case "validator-status":
		handleValidatorStatus()
	case "suspend":
		handleSuspend()
	case "slash":
		handleSlash()
	case "show-econ":
		handleShowEcon()

	default:
		fmt.Println("Unknown command:", cmd)
		printUsage()
	}
}

// ================= FUNGSI UTILITY =================
func printUsage() {
	fmt.Println("Usage: hyperlux -[command] [arguments]")
	fmt.Println("Commands:")
	fmt.Println(" - init                   - Inisialisasi ledger baru")
	fmt.Println(" - start                  - Memulai node dan sinkronisasi (consensus auto-producer aktif)")
	fmt.Println(" - tx send <to> <amount> <walletfile>")
	fmt.Println(" - tx-bulk <count> <to> <walletfile>")
	fmt.Println(" - tx-bulk-random-parallel <count> <walletfile> <workers>")
	fmt.Println(" - tx-bulk-multi <walletCount> <perWallet>")
	fmt.Println(" - stress-test <walletCount> <perWallet> <rounds> <intervalSeconds>")
	fmt.Println(" - auto-commit <seconds>")
	fmt.Println(" - stress-and-commit <walletCount> <perWallet> <intervalSeconds>")
	fmt.Println(" - wallet-bulk <count>")
	fmt.Println(" - metrics                - Menampilkan metrik blockchain")
	fmt.Println(" - commit                 - Memaksa commit block sekali")
	fmt.Println(" - airdrop <amount> <folder>")
	fmt.Println(" - fix-validators         - Memperbaiki data validator")
	fmt.Println(" - full-test <walletCount> <perWallet> <intervalSeconds>")
	fmt.Println("")
	fmt.Println("Validator & Security:")
	fmt.Println(" - validator-status <address>")
	fmt.Println(" - suspend <address> <scope:propose|vote|all> <duration:e.g. 15m,2h,24h>")
	fmt.Println(" - slash <address> <amount> [reporterAddress]")
	fmt.Println(" - show-econ              - Tampilkan treasury, burned, total stake, dsb.")
}

// Pastikan validator & wallet validator tersedia di memori (tanpa start consensus producer)
func ensureValidatorsReady() {
	ledger.LoadValidators()
	ledger.AutoLoadValidatorWallets()
}

// ================= HANDLERS UNTUK SETIAP PERINTAH =================
func handleInit() {
	ledger.InitLedger()
}

func handleStart() {
	network.InitNetwork()
	consensus.InitConsensus() // ini juga akan AutoLoad validator wallets & start block producer
	fmt.Println("✅ Node is running...")
}

func handleTx() {
	if len(os.Args) < 6 || os.Args[2] != "send" {
		fmt.Println("Usage: hyperlux -tx send <to> <amount> <walletfile>")
		return
	}
	to := os.Args[3]
	amount, err := strconv.Atoi(os.Args[4])
	if err != nil {
		log.Fatal("❌ Jumlah transaksi tidak valid:", err)
	}
	walletFile := os.Args[5]

	w, err := wallet.LoadWallet(walletFile)
	if err != nil {
		log.Fatal("❌ Gagal load wallet:", err)
	}

	tx := ledger.NewTransaction(w, to, amount)
	if err := ledger.ValidateAndAddToMempool(tx); err != nil {
		log.Fatal("❌", err)
	}

	hash := ledger.HashTransaction(tx)
	fmt.Println("✅ TX berhasil dikirim")
	fmt.Println("TX Hash:", hash)
}

func handleTxBulk() {
	if len(os.Args) < 5 {
		fmt.Println("Usage: hyperlux -tx-bulk <count> <to> <walletfile>")
		return
	}
	count, _ := strconv.Atoi(os.Args[2])
	to := os.Args[3]
	walletFile := os.Args[4]

	w, err := wallet.LoadWallet(walletFile)
	if err != nil {
		log.Fatal("❌ Gagal load wallet:", err)
	}

	for i := 0; i < count; i++ {
		tx := ledger.NewTransaction(w, to, 1)
		if err := ledger.ValidateAndAddToMempool(tx); err != nil {
			log.Fatal("❌", err)
		}
	}
	fmt.Printf("✅ %d transaksi berhasil ditambahkan ke mempool\n", count)
}

func handleTxBulkRandomParallel() {
	if len(os.Args) < 5 {
		fmt.Println("Usage: hyperlux -tx-bulk-random-parallel <count> <walletfile> <workers>")
		return
	}
	count, _ := strconv.Atoi(os.Args[2])
	walletFile := os.Args[3]
	workers, _ := strconv.Atoi(os.Args[4])

	w, err := wallet.LoadWallet(walletFile)
	if err != nil {
		log.Fatal("❌ Gagal load wallet:", err)
	}

	fmt.Printf("🚀 Mulai parallel TX generator: total=%d, workers=%d\n", count, workers)

	jobs := make(chan int, count)
	go func() {
		for i := 0; i < count; i++ {
			jobs <- i
		}
		close(jobs)
	}()

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for range jobs {
				rnd := make([]byte, 4)
				rand.Read(rnd)
				toAddr := "hlcRnd" + hex.EncodeToString(rnd)
				tx := ledger.NewTransaction(w, toAddr, 1)
				_ = ledger.ValidateAndAddToMempool(tx)
			}
		}()
	}
	wg.Wait()

	fmt.Printf("✅ %d TX berhasil dimasukkan ke mempool secara paralel\n", count)
}

func handleTxBulkMulti() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: hyperlux -tx-bulk-multi <walletCount> <perWallet>")
		return
	}
	walletCount, _ := strconv.Atoi(os.Args[2])
	perWallet, _ := strconv.Atoi(os.Args[3])

	files, _ := os.ReadDir("bulk-wallets")
	if len(files) < walletCount {
		log.Fatalf("❌ Hanya ada %d wallet di bulk-wallets, butuh %d\n", len(files), walletCount)
	}

	var wg sync.WaitGroup
	wg.Add(walletCount)

	for i := 0; i < walletCount; i++ {
		go func(id int) {
			defer wg.Done()
			w, _ := wallet.LoadWallet("bulk-wallets/" + files[id].Name())
			for j := 0; j < perWallet; j++ {
				rnd := make([]byte, 4)
				rand.Read(rnd)
				toAddr := "hlcRnd" + hex.EncodeToString(rnd)
				tx := ledger.NewTransaction(w, toAddr, 1)
				_ = ledger.ValidateAndAddToMempool(tx)
			}
		}(i)
	}
	wg.Wait()

	fmt.Printf("🚀 Mulai spam: %d wallet × %d TX = %d total TX\n", walletCount, perWallet, walletCount*perWallet)
}

func handleStressTest() {
	// Usage: stress-test <walletCount> <perWallet> <rounds> <intervalSeconds>
	if len(os.Args) < 6 {
		fmt.Println("Usage: hyperlux -stress-test <walletCount> <perWallet> <rounds> <intervalSeconds>")
		return
	}
	walletCount, _ := strconv.Atoi(os.Args[2])
	perWallet, _ := strconv.Atoi(os.Args[3])
	maxRounds, _ := strconv.Atoi(os.Args[4])
	interval, _ := strconv.Atoi(os.Args[5])

	// Pastikan validator & wallet validator siap
	ensureValidatorsReady()

	fmt.Printf("🚀 Stress test started: %d wallet × %d TX per round, commit every %ds, for %d rounds\n",
		walletCount, perWallet, interval, maxRounds)

	for i := 0; i < maxRounds; i++ {
		fmt.Printf("--- Round %d/%d ---\n", i+1, maxRounds)

		files, err := os.ReadDir("bulk-wallets")
		if err != nil || len(files) == 0 {
			log.Fatal("❌ Tidak ada wallet di bulk-wallets/. Jalankan dulu wallet-bulk.")
		}

		if walletCount > len(files) {
			walletCount = len(files)
		}

		var wg sync.WaitGroup
		jobs := make(chan string, walletCount)

		go func() {
			for j := 0; j < walletCount; j++ {
				jobs <- "bulk-wallets/" + files[j].Name()
			}
			close(jobs)
		}()

		workers := 10
		wg.Add(workers)
		for wID := 0; wID < workers; wID++ {
			go func() {
				defer wg.Done()
				for file := range jobs {
					w, err := wallet.LoadWallet(file)
					if err != nil {
						fmt.Println("❌ gagal load wallet:", file, err)
						continue
					}
					for j := 0; j < perWallet; j++ {
						rnd := make([]byte, 4)
						rand.Read(rnd)
						toAddr := "hlcRnd" + hex.EncodeToString(rnd)
						tx := ledger.NewTransaction(w, toAddr, 1)
						_ = ledger.ValidateAndAddToMempool(tx)
					}
				}
			}()
		}
		wg.Wait()

		consensus.CommitBlock()
		ledger.SaveAllData()
		time.Sleep(time.Duration(interval) * time.Second)
	}
	fmt.Println("✅ Stress test selesai.")
}

func handleAutoCommit() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: hyperlux -auto-commit <seconds>")
		return
	}
	interval, _ := strconv.Atoi(os.Args[2])

	// Pastikan validator & wallet validator siap
	ensureValidatorsReady()

	fmt.Printf("⚡ Auto-commit enabled (interval %ds)\n", interval)

	for {
		consensus.CommitBlock()
		ledger.SaveAllData()
		time.Sleep(time.Duration(interval) * time.Second)
	}
}

func handleStressAndCommit() {
	// Usage: stress-and-commit <walletCount> <perWallet> <intervalSeconds>
	if len(os.Args) < 5 {
		fmt.Println("Usage: hyperlux -stress-and-commit <walletCount> <perWallet> <intervalSeconds>")
		return
	}
	walletCount, _ := strconv.Atoi(os.Args[2])
	perWallet, _ := strconv.Atoi(os.Args[3])
	interval, _ := strconv.Atoi(os.Args[4])

	// Pastikan validator & wallet validator siap
	ensureValidatorsReady()

	// Jalankan auto-commit di goroutine terpisah
	go func() {
		for {
			consensus.CommitBlock()
			ledger.SaveAllData()
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}()

	// Jalankan stress-test di goroutine utama
	const maxRounds = 5
	fmt.Printf("🚀 Stress test started: %d wallet × %d TX per round, commit every %ds, for %d rounds\n",
		walletCount, perWallet, interval, maxRounds)

	for i := 0; i < maxRounds; i++ {
		fmt.Printf("--- Round %d/%d ---\n", i+1, maxRounds)

		files, err := os.ReadDir("bulk-wallets")
		if err != nil || len(files) == 0 {
			log.Fatal("❌ Tidak ada wallet di bulk-wallets/. Jalankan dulu wallet-bulk.")
		}

		if walletCount > len(files) {
			walletCount = len(files)
		}

		var wg sync.WaitGroup
		jobs := make(chan string, walletCount)

		go func() {
			for j := 0; j < walletCount; j++ {
				jobs <- "bulk-wallets/" + files[j].Name()
			}
			close(jobs)
		}()

		workers := 10
		wg.Add(workers)
		for wID := 0; wID < workers; wID++ {
			go func() {
				defer wg.Done()
				for file := range jobs {
					w, err := wallet.LoadWallet(file)
					if err != nil {
						fmt.Println("❌ gagal load wallet:", file, err)
						continue
					}
					for j := 0; j < perWallet; j++ {
						rnd := make([]byte, 4)
						rand.Read(rnd)
						toAddr := "hlcRnd" + hex.EncodeToString(rnd)
						tx := ledger.NewTransaction(w, toAddr, 1)
						_ = ledger.ValidateAndAddToMempool(tx)
					}
				}
			}()
		}
		wg.Wait()
		time.Sleep(time.Duration(interval) * time.Second)
	}
	fmt.Println("✅ Stress test selesai.")
}

func handleWalletBulk() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: hyperlux -wallet-bulk <count>")
		return
	}
	count, _ := strconv.Atoi(os.Args[2])
	_ = os.MkdirAll("bulk-wallets", 0755)

	for i := 0; i < count; i++ {
		w := wallet.GenerateWallet()
		filename := fmt.Sprintf("bulk-wallets/wallet%d.json", i)
		w.SaveToFile(filename)
	}

	fmt.Printf("✅ %d wallet baru dibuat di folder bulk-wallets/\n", count)
}

func handleMetrics() {
	if len(ledger.Blockchain) == 0 {
		fmt.Println("⚠️ No blocks found")
		return
	}

	height := len(ledger.Blockchain)
	last := ledger.Blockchain[height-1]
	fmt.Printf("⛓  Chain Height : %d\n", height)
	fmt.Printf("🧱 Last Block    : #%d  (hash=%.12s...)\n", last.Index, last.Hash)
	fmt.Printf("   TX in Block   : %d\n", len(last.Transactions))

	// Block time: selisih dua blok terakhir
	if height >= 2 {
		prev := ledger.Blockchain[height-2]
		dt := last.Timestamp - prev.Timestamp
		if dt <= 0 {
			dt = 1
		}
		fmt.Printf("⏱  Block Time    : %ds (last)\n", dt)
	} else {
		fmt.Println("⏱  Block Time    : n/a (genesis only)")
	}

	// TPS berbasis window blok terakhir (mis. 20)
	window := 20
	if height-1 < window {
		window = height - 1
	}
	var sumTx int
	var sumDt int64
	if window > 0 {
		start := height - 1 - window
		for i := start + 1; i < height; i++ {
			b := ledger.Blockchain[i]
			p := ledger.Blockchain[i-1]
			sumTx += len(b.Transactions)
			d := b.Timestamp - p.Timestamp
			if d > 0 {
				sumDt += d
			}
		}
	}
	if sumDt <= 0 {
		sumDt = 1
	}
	tpsRecent := float64(sumTx) / float64(sumDt)
	fmt.Printf("🚀 TPS (last %d blocks): %.2f\n", window, tpsRecent)

	// Mempool + runtime profiling singkat
	mp := ledger.GetMempoolSize()
	fmt.Printf("🧺 Mempool Size : %d\n", mp)

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("🧵 Goroutines    : %d\n", runtime.NumGoroutine())
	fmt.Printf("🧠 Memory        : %.2f MiB (Alloc), %.2f MiB (Sys)\n",
		float64(m.Alloc)/1024.0/1024.0,
		float64(m.Sys)/1024.0/1024.0,
	)

	fmt.Printf("✅ Finality      : %s\n", consensus.GetFinalityStatus())
}

func handleCommit() {
	// Pastikan validator & wallet validator siap
	ensureValidatorsReady()

	consensus.CommitBlock()
	ledger.SaveAllData()
}

func handleAirdrop() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: hyperlux -airdrop <amount> <folder>")
		fmt.Println("Example: hyperlux -airdrop 10000 bulk-wallets")
		return
	}
	amount, _ := strconv.Atoi(os.Args[2])
	folder := os.Args[3]

	files, err := os.ReadDir(folder)
	if err != nil {
		log.Fatal("❌ gagal baca folder:", err)
	}

	for _, f := range files {
		w, err := wallet.LoadWallet(folder + "/" + f.Name())
		if err != nil {
			fmt.Println("❌ gagal load wallet:", f.Name(), err)
			continue
		}
		ledger.Balances[w.AddressEd] += amount
		fmt.Printf("💸 Airdrop %d ke %s\n", amount, w.AddressEd)
	}

	ledger.SaveBalances()
	fmt.Printf("✅ Airdrop selesai ke %d wallet (masing-masing %d)\n", len(files), amount)
}

func handleFixValidators() {
	ledger.FixValidators()
	ledger.SaveAllData()
}

func handleFullTest() {
	// Usage: full-test <walletCount> <perWallet> <intervalSeconds>
	if len(os.Args) < 5 {
		fmt.Println("Usage: hyperlux -full-test <walletCount> <perWallet> <intervalSeconds>")
		return
	}

	fmt.Println("🚀 Menyiapkan dan memulai pengujian penuh...")

	// 1) Sinkronisasi & siapkan validator + wallet validators di memori
	ledger.FixValidators()            // buat/repair daftar validator & file wallet validator
	ledger.LoadValidators()           // muat validator ke memori
	ledger.AutoLoadValidatorWallets() // muat wallet validator (agar CommitBlock tidak gagal)
	ledger.SaveAllData()
	fmt.Println("✅ Validator berhasil didaftarkan dan disimpan.")

	// 2) Ambil parameter
	walletCount, _ := strconv.Atoi(os.Args[2])
	perWallet, _ := strconv.Atoi(os.Args[3])
	interval, _ := strconv.Atoi(os.Args[4])

	// 3) Jalankan auto-commit di goroutine terpisah (tanpa InitConsensus agar tidak dobel producer)
	go func() {
		for {
			consensus.CommitBlock()
			ledger.SaveAllData()
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}()

	// 4) Jalankan stress-test
	const maxRounds = 5
	fmt.Printf("🚀 Stress test started: %d wallet × %d TX per round, commit every %ds, for %d rounds\n",
		walletCount, perWallet, interval, maxRounds)

	for i := 0; i < maxRounds; i++ {
		fmt.Printf("--- Round %d/%d ---\n", i+1, maxRounds)

		files, err := os.ReadDir("bulk-wallets")
		if err != nil || len(files) == 0 {
			log.Fatal("❌ Tidak ada wallet di bulk-wallets/. Jalankan dulu wallet-bulk.")
		}
		if walletCount > len(files) {
			walletCount = len(files)
		}

		var wg sync.WaitGroup
		jobs := make(chan string, walletCount)
		go func() {
			for j := 0; j < walletCount; j++ {
				jobs <- "bulk-wallets/" + files[j].Name()
			}
			close(jobs)
		}()
		workers := 10
		wg.Add(workers)
		for wID := 0; wID < workers; wID++ {
			go func() {
				defer wg.Done()
				for file := range jobs {
					w, _ := wallet.LoadWallet(file)
					for j := 0; j < perWallet; j++ {
						rnd := make([]byte, 4)
						rand.Read(rnd)
						toAddr := "hlcRnd" + hex.EncodeToString(rnd)
						tx := ledger.NewTransaction(w, toAddr, 1)
						_ = ledger.ValidateAndAddToMempool(tx)
					}
				}
			}()
		}
		wg.Wait()
		time.Sleep(time.Duration(interval) * time.Second)
	}
	fmt.Println("✅ Stress test selesai.")
}

// ===================== VALIDATOR SECURITY COMMANDS =====================

func handleValidatorStatus() {
	// Usage: validator-status <address>
	if len(os.Args) < 3 {
		fmt.Println("Usage: hyperlux -validator-status <address>")
		return
	}
	addr := os.Args[2]
	ensureValidatorsReady()

	// cari validator
	found := false
	stake := 0
	for _, v := range ledger.Validators {
		if v.Address == addr {
			found = true
			stake = v.Stake
			break
		}
	}
	if !found {
		fmt.Printf("❌ %s tidak terdaftar sebagai validator\n", addr)
		return
	}

	// status suspend
	sProp := ledger.IsSuspended(addr, ledger.ScopePropose)
	sVote := ledger.IsSuspended(addr, ledger.ScopeVote)
	sAll := ledger.IsSuspended(addr, ledger.ScopeAll)

	// baca runtime record untuk detail until/scope
	ledger.ValidatorStatusMu.RLock()
	rt := ledger.ValidatorStatus[addr]
	ledger.ValidatorStatusMu.RUnlock()

	var untilStr, scopeStr string
	if rt != nil && time.Now().Unix() < rt.SuspendedUntil {
		untilStr = time.Unix(rt.SuspendedUntil, 0).Format(time.RFC3339)
		scopeStr = scopeToString(rt.SuspendScope)
	} else {
		untilStr = "-"
		scopeStr = "-"
	}

	// wallet
	_, hasWallet := ledger.ValidatorWallets[addr]

	fmt.Println("🧾 Validator Status")
	fmt.Println("-------------------")
	fmt.Printf("Address          : %s\n", addr)
	fmt.Printf("Stake            : %d\n", stake)
	fmt.Printf("Wallet Loaded    : %v\n", hasWallet)
	fmt.Printf("Suspended(Propose): %v\n", sProp)
	fmt.Printf("Suspended(Vote)   : %v\n", sVote)
	fmt.Printf("Suspended(All)    : %v\n", sAll)
	fmt.Printf("Suspension Scope  : %s\n", scopeStr)
	fmt.Printf("Suspended Until   : %s\n", untilStr)
}

func handleSuspend() {
	// Usage: suspend <address> <scope:propose|vote|all> <duration:15m|2h|24h>
	if len(os.Args) < 5 {
		fmt.Println("Usage: hyperlux -suspend <address> <scope:propose|vote|all> <duration>")
		return
	}
	addr := os.Args[2]
	scopeStr := strings.ToLower(os.Args[3])
	durStr := os.Args[4]

	scope, ok := parseScope(scopeStr)
	if !ok {
		fmt.Println("❌ scope harus salah satu dari: propose, vote, all")
		return
	}
	dur, err := time.ParseDuration(durStr)
	if err != nil {
		fmt.Println("❌ duration invalid. contoh: 15m, 2h, 24h")
		return
	}

	ledger.SuspendValidator(addr, scope, dur)
	fmt.Printf("✅ %s disuspend scope=%s durasi=%s\n", addr, scopeStr, durStr)
}

func handleSlash() {
	// Usage: slash <address> <amount> [reporterAddress]
	if len(os.Args) < 4 {
		fmt.Println("Usage: hyperlux -slash <address> <amount> [reporterAddress]")
		return
	}
	addr := os.Args[2]
	amount, err := strconv.Atoi(os.Args[3])
	if err != nil || amount <= 0 {
		fmt.Println("❌ amount harus bilangan bulat > 0")
		return
	}
	reporter := ""
	if len(os.Args) >= 5 && os.Args[4] != "-" {
		reporter = os.Args[4]
	}

	ensureValidatorsReady()

	ledger.SlashSafetyFault(addr, amount, reporter, 1.0)
	// persist perubahan stake + distribusi hadiah ke balance
	ledger.SaveValidators() // <— HILANGKAN `_ =`
	ledger.SaveBalances()

	fmt.Printf("✅ Slash sukses. Offender=%s amount=%d reporter=%s\n", addr, amount, reporter)
}

func handleShowEcon() {
	fmt.Println("💰 Economic Metrics")
	fmt.Println("-------------------")

	// Treasury & Burned
	fmt.Printf("Treasury Balance : %d\n", ledger.TreasuryBalance)
	fmt.Printf("Burned Supply    : %d\n", ledger.BurnedSupply)

	// Total validator stake
	totalStake := 0
	maxStake := 0
	maxAddr := ""
	for _, v := range ledger.Validators {
		totalStake += v.Stake
		if v.Stake > maxStake {
			maxStake = v.Stake
			maxAddr = v.Address
		}
	}
	fmt.Printf("Total Validators : %d\n", len(ledger.Validators))
	fmt.Printf("Total Stake      : %d\n", totalStake)
	if maxAddr != "" {
		fmt.Printf("Top Validator    : %s (stake=%d)\n", maxAddr, maxStake)
	}

	// (Opsional) tampilkan beberapa saldo validator sebagai indikasi redistribusi honest
	showN := 5
	if showN > len(ledger.Validators) {
		showN = len(ledger.Validators)
	}
	if showN > 0 {
		fmt.Println("Sample Balances (validator):")
		for i := 0; i < showN; i++ {
			addr := ledger.Validators[i].Address
			ledger.BalanceMu.RLock()
			bal := ledger.Balances[addr]
			ledger.BalanceMu.RUnlock()
			fmt.Printf(" - %s : %d\n", addr, bal)
		}
	}
}

// ------------------- helpers -------------------

func parseScope(s string) (ledger.SuspensionScope, bool) {
	switch strings.ToLower(s) {
	case "propose":
		return ledger.ScopePropose, true
	case "vote":
		return ledger.ScopeVote, true
	case "all":
		return ledger.ScopeAll, true
	default:
		return ledger.ScopeNone, false
	}
}

func scopeToString(sc ledger.SuspensionScope) string {
	switch sc {
	case ledger.ScopePropose:
		return "propose"
	case ledger.ScopeVote:
		return "vote"
	case ledger.ScopeAll:
		return "all"
	default:
		return "none"
	}
}
