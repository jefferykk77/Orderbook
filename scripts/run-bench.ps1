param(
    [ValidateRange(1, 3600)]
    [int]$Seconds = 1,

    [ValidateRange(1, 30)]
    [int]$Count = 3,

    [switch]$Profile
)

$ErrorActionPreference = "Stop"

$moduleRoot = Split-Path -Parent $PSScriptRoot
$outputRoot = Join-Path $moduleRoot "benchmarks"
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$outputDir = Join-Path $outputRoot $timestamp

New-Item -ItemType Directory -Force -Path $outputDir | Out-Null

$resultPath = Join-Path $outputDir "bench.txt"
$benchmarks = @(
    "BenchmarkLimitOrderNoCross",
    "BenchmarkLimitOrderMatch",
    "BenchmarkMarketOrderPartialFill",
    "BenchmarkCalculateMarketPrice",
    "BenchmarkDepth",
    "BenchmarkMarshalJSON",
    "BenchmarkParallelIndependentBooks",
    "BenchmarkBookString"
)
$joinedBenchmarks = ($benchmarks | ForEach-Object { $_.Substring("Benchmark".Length) }) -join "|"
$benchmarkRegex = "^Benchmark($joinedBenchmarks)$"

function Invoke-RecordedCommand {
    param(
        [string]$FilePath,
        [scriptblock]$Command
    )

    $output = & $Command 2>&1
    if ($null -ne $output) {
        Add-Content -Encoding UTF8 -Path $FilePath -Value $output
    }
    return $output
}

Push-Location $moduleRoot
try {
    $cpu = Get-CimInstance Win32_Processor |
        Select-Object Name, NumberOfCores, NumberOfLogicalProcessors |
        Format-List |
        Out-String

    $commit = ""
    try {
        $commit = git rev-parse --short HEAD 2>$null
        if ($LASTEXITCODE -ne 0) {
            $commit = "not-a-git-repository"
        }
    }
    catch {
        $commit = "not-a-git-repository"
    }

    @"
Timestamp: $(Get-Date -Format o)
Module: $moduleRoot
Commit: $commit
GOMAXPROCS(env): $env:GOMAXPROCS
Logical processors reported by OS: $env:NUMBER_OF_PROCESSORS
$cpu
Command: go test -run '^$' -bench '$benchmarkRegex' -benchmem -benchtime ${Seconds}s -count $Count
"@ | Set-Content -Encoding UTF8 -Path $resultPath

    "go version".PadRight(80, "=") | Add-Content -Encoding UTF8 -Path $resultPath
    Invoke-RecordedCommand -FilePath $resultPath -Command { go version }
    if ($LASTEXITCODE -ne 0) {
        throw "go version failed"
    }

    "go env".PadRight(80, "=") | Add-Content -Encoding UTF8 -Path $resultPath
    Invoke-RecordedCommand -FilePath $resultPath -Command { go env GOOS GOARCH GOVERSION CGO_ENABLED }
    if ($LASTEXITCODE -ne 0) {
        throw "go env failed"
    }

    "unit tests".PadRight(80, "=") | Add-Content -Encoding UTF8 -Path $resultPath
    Invoke-RecordedCommand -FilePath $resultPath -Command { go test ./... }
    if ($LASTEXITCODE -ne 0) {
        throw "go test failed"
    }

    "benchmarks".PadRight(80, "=") | Add-Content -Encoding UTF8 -Path $resultPath
    Invoke-RecordedCommand -FilePath $resultPath -Command {
        go test -run '^$' -bench $benchmarkRegex -benchmem -benchtime "${Seconds}s" -count $Count
    }
    if ($LASTEXITCODE -ne 0) {
        throw "benchmark run failed"
    }

    if ($Profile) {
        $profileDir = Join-Path $outputDir "profiles"
        New-Item -ItemType Directory -Force -Path $profileDir | Out-Null
        $testBinary = Join-Path $profileDir "orderbook.test.exe"

        Invoke-RecordedCommand -FilePath $resultPath -Command {
            go test -c -o $testBinary
        }
        if ($LASTEXITCODE -ne 0) {
            throw "profile test binary build failed"
        }

        Invoke-RecordedCommand -FilePath $resultPath -Command {
            & $testBinary "-test.run=^$" "-test.bench=^BenchmarkLimitOrderMatch$" "-test.benchmem=true" "-test.benchtime=${Seconds}s" "-test.count=1" "-test.cpuprofile=cpu.out" "-test.memprofile=mem.out" "-test.outputdir=$profileDir"
        }
        if ($LASTEXITCODE -ne 0) {
            throw "profile benchmark failed"
        }
    }
}
finally {
    Pop-Location
}

Write-Host "Benchmark results written to $resultPath"
