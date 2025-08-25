package main

import (
	"log"
	"os"
	"runtime/pprof"

	"github.com/soden46/hyperlux-chain/cli"
)

func main() {
	// ---------------------- Profiling CPU ----------------------
	// Buat file untuk menyimpan data profil CPU
	cpuProfileFile, err := os.Create("cpu.prof")
	if err != nil {
		log.Fatal("Tidak bisa membuat file CPU profile: ", err)
	}
	defer cpuProfileFile.Close()

	// Mulai profiling CPU
	if err := pprof.StartCPUProfile(cpuProfileFile); err != nil {
		log.Fatal("Tidak bisa memulai CPU profile: ", err)
	}
	defer pprof.StopCPUProfile()

	// ---------------------- Jalankan Program Utama ----------------------
	cli.RunCLI()

	// ---------------------- Profiling Memori ----------------------
	// Buat file untuk menyimpan data profil memori
	memProfileFile, err := os.Create("mem.prof")
	if err != nil {
		log.Fatal("Tidak bisa membuat file memory profile: ", err)
	}
	defer memProfileFile.Close()

	// Tulis data profil memori ke file
	if err := pprof.WriteHeapProfile(memProfileFile); err != nil {
		log.Fatal("Tidak bisa menulis heap profile: ", err)
	}
}