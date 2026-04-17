# sshexec_go

A batch SSH command execution tool written in Go.

## Project Overview

sshexec_go is an efficient batch SSH command execution tool that supports concurrent command execution, result collection, error classification, and logging. It is designed for executing operational tasks across multiple remote servers.

## Main Features

- **Batch Execution**: Supports executing SSH commands across multiple servers simultaneously.
- **Concurrency Control**: Configurable maximum concurrency to effectively manage resource usage.
- **Result Handling**: Automatically collects and processes command execution results.
- **Error Classification**: Automatically identifies and categorizes common SSH execution errors.
- **Logging**: Supports logging with success-level markers.
- **Interrupt Handling**: Supports graceful handling of interrupt signals.
- **JSON Configuration**: Supports task parameter configuration via JSON format.

## Project Structure

```
sshexec_go/
├── main.go                          # Program entry point
├── internal/
│   ├── core/
│   │   ├── batch_executor.go       # Batch executor
│   │   └── result_consumer.go      # Result consumer
│   ├── interrupt/
│   │   └── interrupt.go            # Interrupt handler
│   ├── jsonproc/
│   │   ├── json_parser.go          # JSON parser
│   │   └── json_type.go            # JSON data type definitions
│   ├── log/
│   │   ├── logger.go               # Logger
│   │   └── successLevel.go         # Success-level logging
│   ├── socket/
│   │   └── client.go               # Socket client
│   └── ssh/
│       ├── ssh_result.go           # SSH execution result
│       └── ssh_run.go              # SSH client implementation
```

## Core Components

### BatchExecutor
Responsible for batch execution of SSH tasks with configurable concurrency and real-time progress reporting.

### ResultConsumer
Collects and processes command execution results, supporting formatted output to a specified directory.

### SSHClient
SSH connection client supporting key-based authentication and command execution, with timeout and cancellation support.

### InterruptHandler
Handles interrupt signals and provides cancellable context.

## Usage

### Configuration File Format

The program supports JSON-formatted configuration files, primarily including:
- Node information (IP, port, username, etc.)
- Commands to execute
- Authentication configuration

### Running the Program

```bash
./sshexec_go -config config.json
```

For detailed command-line arguments, refer to the program's help information.

## Dependencies

- Go 1.16+
- golang.org/x/crypto/ssh
- go.uber.org/zap

## License

Please refer to the LICENSE file in the project for detailed information.