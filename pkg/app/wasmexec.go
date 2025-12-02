//go:build !wasm

package app

import (
	"runtime"
	"strconv"
	"strings"
)

const (
	wasmExecTinyGo = `
// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// This file has been modified for use by the TinyGo compiler.

(() => {
	// Map multiple JavaScript environments to a single common API,
	// preferring web standards over Node.js API.
	//
	// Environments considered:
	// - Browsers
	// - Node.js
	// - Electron
	// - Parcel

	if (typeof global !== "undefined") {
		// global already exists
	} else if (typeof window !== "undefined") {
		window.global = window;
	} else if (typeof self !== "undefined") {
		self.global = self;
	} else {
		throw new Error("cannot export Go (neither global, window nor self is defined)");
	}

	if (!global.require && typeof require !== "undefined") {
		global.require = require;
	}

	if (!global.fs && global.require) {
		global.fs = require("node:fs");
	}

	const enosys = () => {
		const err = new Error("not implemented");
		err.code = "ENOSYS";
		return err;
	};

	if (!global.fs) {
		let outputBuf = "";
		global.fs = {
			constants: { O_WRONLY: -1, O_RDWR: -1, O_CREAT: -1, O_TRUNC: -1, O_APPEND: -1, O_EXCL: -1 }, // unused
			writeSync(fd, buf) {
				outputBuf += decoder.decode(buf);
				const nl = outputBuf.lastIndexOf("\n");
				if (nl != -1) {
					console.log(outputBuf.substr(0, nl));
					outputBuf = outputBuf.substr(nl + 1);
				}
				return buf.length;
			},
			write(fd, buf, offset, length, position, callback) {
				if (offset !== 0 || length !== buf.length || position !== null) {
					callback(enosys());
					return;
				}
				const n = this.writeSync(fd, buf);
				callback(null, n);
			},
			chmod(path, mode, callback) { callback(enosys()); },
			chown(path, uid, gid, callback) { callback(enosys()); },
			close(fd, callback) { callback(enosys()); },
			fchmod(fd, mode, callback) { callback(enosys()); },
			fchown(fd, uid, gid, callback) { callback(enosys()); },
			fstat(fd, callback) { callback(enosys()); },
			fsync(fd, callback) { callback(null); },
			ftruncate(fd, length, callback) { callback(enosys()); },
			lchown(path, uid, gid, callback) { callback(enosys()); },
			link(path, link, callback) { callback(enosys()); },
			lstat(path, callback) { callback(enosys()); },
			mkdir(path, perm, callback) { callback(enosys()); },
			open(path, flags, mode, callback) { callback(enosys()); },
			read(fd, buffer, offset, length, position, callback) { callback(enosys()); },
			readdir(path, callback) { callback(enosys()); },
			readlink(path, callback) { callback(enosys()); },
			rename(from, to, callback) { callback(enosys()); },
			rmdir(path, callback) { callback(enosys()); },
			stat(path, callback) { callback(enosys()); },
			symlink(path, link, callback) { callback(enosys()); },
			truncate(path, length, callback) { callback(enosys()); },
			unlink(path, callback) { callback(enosys()); },
			utimes(path, atime, mtime, callback) { callback(enosys()); },
		};
	}

	if (!global.process) {
		global.process = {
			getuid() { return -1; },
			getgid() { return -1; },
			geteuid() { return -1; },
			getegid() { return -1; },
			getgroups() { throw enosys(); },
			pid: -1,
			ppid: -1,
			umask() { throw enosys(); },
			cwd() { throw enosys(); },
			chdir() { throw enosys(); },
		}
	}

	if (!global.crypto) {
		const nodeCrypto = require("node:crypto");
		global.crypto = {
			getRandomValues(b) {
				nodeCrypto.randomFillSync(b);
			},
		};
	}

	if (!global.performance) {
		global.performance = {
			now() {
				const [sec, nsec] = process.hrtime();
				return sec * 1000 + nsec / 1000000;
			},
		};
	}

	if (!global.TextEncoder) {
		global.TextEncoder = require("node:util").TextEncoder;
	}

	if (!global.TextDecoder) {
		global.TextDecoder = require("node:util").TextDecoder;
	}

	// End of polyfills for common API.

	const encoder = new TextEncoder("utf-8");
	const decoder = new TextDecoder("utf-8");
	let reinterpretBuf = new DataView(new ArrayBuffer(8));
	var logLine = [];
	const wasmExit = {}; // thrown to exit via proc_exit (not an error)

	global.Go = class {
		constructor() {
			this._callbackTimeouts = new Map();
			this._nextCallbackTimeoutID = 1;

			const mem = () => {
				// The buffer may change when requesting more memory.
				return new DataView(this._inst.exports.memory.buffer);
			}

			const unboxValue = (v_ref) => {
				reinterpretBuf.setBigInt64(0, v_ref, true);
				const f = reinterpretBuf.getFloat64(0, true);
				if (f === 0) {
					return undefined;
				}
				if (!isNaN(f)) {
					return f;
				}

				const id = v_ref & 0xffffffffn;
				return this._values[id];
			}


			const loadValue = (addr) => {
				let v_ref = mem().getBigUint64(addr, true);
				return unboxValue(v_ref);
			}

			const boxValue = (v) => {
				const nanHead = 0x7FF80000n;

				if (typeof v === "number") {
					if (isNaN(v)) {
						return nanHead << 32n;
					}
					if (v === 0) {
						return (nanHead << 32n) | 1n;
					}
					reinterpretBuf.setFloat64(0, v, true);
					return reinterpretBuf.getBigInt64(0, true);
				}

				switch (v) {
					case undefined:
						return 0n;
					case null:
						return (nanHead << 32n) | 2n;
					case true:
						return (nanHead << 32n) | 3n;
					case false:
						return (nanHead << 32n) | 4n;
				}

				let id = this._ids.get(v);
				if (id === undefined) {
					id = this._idPool.pop();
					if (id === undefined) {
						id = BigInt(this._values.length);
					}
					this._values[id] = v;
					this._goRefCounts[id] = 0;
					this._ids.set(v, id);
				}
				this._goRefCounts[id]++;
				let typeFlag = 1n;
				switch (typeof v) {
					case "string":
						typeFlag = 2n;
						break;
					case "symbol":
						typeFlag = 3n;
						break;
					case "function":
						typeFlag = 4n;
						break;
				}
				return id | ((nanHead | typeFlag) << 32n);
			}

			const storeValue = (addr, v) => {
				let v_ref = boxValue(v);
				mem().setBigUint64(addr, v_ref, true);
			}

			const loadSlice = (array, len, cap) => {
				return new Uint8Array(this._inst.exports.memory.buffer, array, len);
			}

			const loadSliceOfValues = (array, len, cap) => {
				const a = new Array(len);
				for (let i = 0; i < len; i++) {
					a[i] = loadValue(array + i * 8);
				}
				return a;
			}

			const loadString = (ptr, len) => {
				return decoder.decode(new DataView(this._inst.exports.memory.buffer, ptr, len));
			}

			const timeOrigin = Date.now() - performance.now();
			this.importObject = {
				wasi_snapshot_preview1: {
					// https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#fd_write
					fd_write: function(fd, iovs_ptr, iovs_len, nwritten_ptr) {
						let nwritten = 0;
						if (fd == 1) {
							for (let iovs_i=0; iovs_i<iovs_len;iovs_i++) {
								let iov_ptr = iovs_ptr+iovs_i*8; // assuming wasm32
								let ptr = mem().getUint32(iov_ptr + 0, true);
								let len = mem().getUint32(iov_ptr + 4, true);
								nwritten += len;
								for (let i=0; i<len; i++) {
									let c = mem().getUint8(ptr+i);
									if (c == 13) { // CR
										// ignore
									} else if (c == 10) { // LF
										// write line
										let line = decoder.decode(new Uint8Array(logLine));
										logLine = [];
										console.log(line);
									} else {
										logLine.push(c);
									}
								}
							}
						} else {
							console.error('invalid file descriptor:', fd);
						}
						mem().setUint32(nwritten_ptr, nwritten, true);
						return 0;
					},
					fd_close: () => 0,      // dummy
					fd_fdstat_get: () => 0, // dummy
					fd_seek: () => 0,       // dummy
					proc_exit: (code) => {
						this.exited = true;
						this.exitCode = code;
						this._resolveExitPromise();
						throw wasmExit;
					},
					random_get: (bufPtr, bufLen) => {
						crypto.getRandomValues(loadSlice(bufPtr, bufLen));
						return 0;
					},
				},
				gojs: {
					// func ticks() int64
					"runtime.ticks": () => {
						return BigInt((timeOrigin + performance.now()) * 1e6);
					},

					// func sleepTicks(timeout int64)
					"runtime.sleepTicks": (timeout) => {
						// Do not sleep, only reactivate scheduler after the given timeout.
						setTimeout(() => {
							if (this.exited) return;
							try {
								this._inst.exports.go_scheduler();
							} catch (e) {
								if (e !== wasmExit) throw e;
							}
						}, Number(timeout)/1e6);
					},

					// func finalizeRef(v ref)
					"syscall/js.finalizeRef": (v_ref) => {
						// Note: TinyGo does not support finalizers so this is only called
						// for one specific case, by js.go:jsString. and can/might leak memory.
						const id = v_ref & 0xffffffffn;
						if (this._goRefCounts?.[id] !== undefined) {
							this._goRefCounts[id]--;
							if (this._goRefCounts[id] === 0) {
								const v = this._values[id];
								this._values[id] = null;
								this._ids.delete(v);
								this._idPool.push(id);
							}
						} else {
							console.error("syscall/js.finalizeRef: unknown id", id);
						}
					},

					// func stringVal(value string) ref
					"syscall/js.stringVal": (value_ptr, value_len) => {
						value_ptr >>>= 0;
						const s = loadString(value_ptr, value_len);
						return boxValue(s);
					},

					// func valueGet(v ref, p string) ref
					"syscall/js.valueGet": (v_ref, p_ptr, p_len) => {
						let prop = loadString(p_ptr, p_len);
						let v = unboxValue(v_ref);
						let result = Reflect.get(v, prop);
						return boxValue(result);
					},

					// func valueSet(v ref, p string, x ref)
					"syscall/js.valueSet": (v_ref, p_ptr, p_len, x_ref) => {
						const v = unboxValue(v_ref);
						const p = loadString(p_ptr, p_len);
						const x = unboxValue(x_ref);
						Reflect.set(v, p, x);
					},

					// func valueDelete(v ref, p string)
					"syscall/js.valueDelete": (v_ref, p_ptr, p_len) => {
						const v = unboxValue(v_ref);
						const p = loadString(p_ptr, p_len);
						Reflect.deleteProperty(v, p);
					},

					// func valueIndex(v ref, i int) ref
					"syscall/js.valueIndex": (v_ref, i) => {
						return boxValue(Reflect.get(unboxValue(v_ref), i));
					},

					// valueSetIndex(v ref, i int, x ref)
					"syscall/js.valueSetIndex": (v_ref, i, x_ref) => {
						Reflect.set(unboxValue(v_ref), i, unboxValue(x_ref));
					},

					// func valueCall(v ref, m string, args []ref) (ref, bool)
					"syscall/js.valueCall": (ret_addr, v_ref, m_ptr, m_len, args_ptr, args_len, args_cap) => {
						const v = unboxValue(v_ref);
						const name = loadString(m_ptr, m_len);
						const args = loadSliceOfValues(args_ptr, args_len, args_cap);
						try {
							const m = Reflect.get(v, name);
							storeValue(ret_addr, Reflect.apply(m, v, args));
							mem().setUint8(ret_addr + 8, 1);
						} catch (err) {
							storeValue(ret_addr, err);
							mem().setUint8(ret_addr + 8, 0);
						}
					},

					// func valueInvoke(v ref, args []ref) (ref, bool)
					"syscall/js.valueInvoke": (ret_addr, v_ref, args_ptr, args_len, args_cap) => {
						try {
							const v = unboxValue(v_ref);
							const args = loadSliceOfValues(args_ptr, args_len, args_cap);
							storeValue(ret_addr, Reflect.apply(v, undefined, args));
							mem().setUint8(ret_addr + 8, 1);
						} catch (err) {
							storeValue(ret_addr, err);
							mem().setUint8(ret_addr + 8, 0);
						}
					},

					// func valueNew(v ref, args []ref) (ref, bool)
					"syscall/js.valueNew": (ret_addr, v_ref, args_ptr, args_len, args_cap) => {
						const v = unboxValue(v_ref);
						const args = loadSliceOfValues(args_ptr, args_len, args_cap);
						try {
							storeValue(ret_addr, Reflect.construct(v, args));
							mem().setUint8(ret_addr + 8, 1);
						} catch (err) {
							storeValue(ret_addr, err);
							mem().setUint8(ret_addr+ 8, 0);
						}
					},

					// func valueLength(v ref) int
					"syscall/js.valueLength": (v_ref) => {
						return unboxValue(v_ref).length;
					},

					// valuePrepareString(v ref) (ref, int)
					"syscall/js.valuePrepareString": (ret_addr, v_ref) => {
						const s = String(unboxValue(v_ref));
						const str = encoder.encode(s);
						storeValue(ret_addr, str);
						mem().setInt32(ret_addr + 8, str.length, true);
					},

					// valueLoadString(v ref, b []byte)
					"syscall/js.valueLoadString": (v_ref, slice_ptr, slice_len, slice_cap) => {
						const str = unboxValue(v_ref);
						loadSlice(slice_ptr, slice_len, slice_cap).set(str);
					},

					// func valueInstanceOf(v ref, t ref) bool
					"syscall/js.valueInstanceOf": (v_ref, t_ref) => {
 						return unboxValue(v_ref) instanceof unboxValue(t_ref);
					},

					// func copyBytesToGo(dst []byte, src ref) (int, bool)
					"syscall/js.copyBytesToGo": (ret_addr, dest_addr, dest_len, dest_cap, src_ref) => {
						let num_bytes_copied_addr = ret_addr;
						let returned_status_addr = ret_addr + 4; // Address of returned boolean status variable

						const dst = loadSlice(dest_addr, dest_len);
						const src = unboxValue(src_ref);
						if (!(src instanceof Uint8Array || src instanceof Uint8ClampedArray)) {
							mem().setUint8(returned_status_addr, 0); // Return "not ok" status
							return;
						}
						const toCopy = src.subarray(0, dst.length);
						dst.set(toCopy);
						mem().setUint32(num_bytes_copied_addr, toCopy.length, true);
						mem().setUint8(returned_status_addr, 1); // Return "ok" status
					},

					// copyBytesToJS(dst ref, src []byte) (int, bool)
					// Originally copied from upstream Go project, then modified:
					//   https://github.com/golang/go/blob/3f995c3f3b43033013013e6c7ccc93a9b1411ca9/misc/wasm/wasm_exec.js#L404-L416
					"syscall/js.copyBytesToJS": (ret_addr, dst_ref, src_addr, src_len, src_cap) => {
						let num_bytes_copied_addr = ret_addr;
						let returned_status_addr = ret_addr + 4; // Address of returned boolean status variable

						const dst = unboxValue(dst_ref);
						const src = loadSlice(src_addr, src_len);
						if (!(dst instanceof Uint8Array || dst instanceof Uint8ClampedArray)) {
							mem().setUint8(returned_status_addr, 0); // Return "not ok" status
							return;
						}
						const toCopy = src.subarray(0, dst.length);
						dst.set(toCopy);
						mem().setUint32(num_bytes_copied_addr, toCopy.length, true);
						mem().setUint8(returned_status_addr, 1); // Return "ok" status
					},
				}
			};

			// Go 1.20 uses 'env'. Go 1.21 uses 'gojs'.
			// For compatibility, we use both as long as Go 1.20 is supported.
			this.importObject.env = this.importObject.gojs;
		}

		async run(instance) {
			this._inst = instance;
			this._values = [ // JS values that Go currently has references to, indexed by reference id
				NaN,
				0,
				null,
				true,
				false,
				global,
				this,
			];
			this._goRefCounts = []; // number of references that Go has to a JS value, indexed by reference id
			this._ids = new Map();  // mapping from JS values to reference ids
			this._idPool = [];      // unused ids that have been garbage collected
			this.exited = false;    // whether the Go program has exited
			this.exitCode = 0;

			if (this._inst.exports._start) {
				let exitPromise = new Promise((resolve, reject) => {
					this._resolveExitPromise = resolve;
				});

				// Run program, but catch the wasmExit exception that's thrown
				// to return back here.
				try {
					this._inst.exports._start();
				} catch (e) {
					if (e !== wasmExit) throw e;
				}

				await exitPromise;
				return this.exitCode;
			} else {
				this._inst.exports._initialize();
			}
		}

		_resume() {
			if (this.exited) {
				throw new Error("Go program has already exited");
			}
			try {
				this._inst.exports.resume();
			} catch (e) {
				if (e !== wasmExit) throw e;
			}
			if (this.exited) {
				this._resolveExitPromise();
			}
		}

		_makeFuncWrapper(id) {
			const go = this;
			return function () {
				const event = { id: id, this: this, args: arguments };
				go._pendingEvent = event;
				go._resume();
				return event.result;
			};
		}
	}

	if (
		global.require &&
		global.require.main === module &&
		global.process &&
		global.process.versions &&
		!global.process.versions.electron
	) {
		if (process.argv.length != 3) {
			console.error("usage: go_js_wasm_exec [wasm binary] [arguments]");
			process.exit(1);
		}

		const go = new Go();
		WebAssembly.instantiate(fs.readFileSync(process.argv[2]), go.importObject).then(async (result) => {
			let exitCode = await go.run(result.instance);
			process.exit(exitCode);
		}).catch((err) => {
			console.error(err);
			process.exit(1);
		});
	}
})();
`
)

func wasmExecJS(tinyGo bool) string {
	if tinyGo {
		return wasmExecTinyGo
	}
	version := strings.Split(runtime.Version(), ".")
	if len(version) < 2 {
		return wasmExecJSGoCurrent
	}

	major, _ := strconv.Atoi(strings.TrimPrefix(version[0], "go"))
	minor, _ := strconv.Atoi(version[1])

	switch {
	case major == 1 && minor < 21:
		return wasmExecJSGo120
	case major == 1 && minor < 24:
		return wasmExecJSGo121
	default:
		return wasmExecJSGoCurrent
	}
}

const (
	wasmExecJSGo121 = "// Copyright 2018 The Go Authors. All rights reserved.\n// Use of this source code is governed by a BSD-style\n// license that can be found in the LICENSE file.\n\n\"use strict\";\n\n(() => {\n\tconst enosys = () => {\n\t\tconst err = new Error(\"not implemented\");\n\t\terr.code = \"ENOSYS\";\n\t\treturn err;\n\t};\n\n\tif (!globalThis.fs) {\n\t\tlet outputBuf = \"\";\n\t\tglobalThis.fs = {\n\t\t\tconstants: { O_WRONLY: -1, O_RDWR: -1, O_CREAT: -1, O_TRUNC: -1, O_APPEND: -1, O_EXCL: -1 }, // unused\n\t\t\twriteSync(fd, buf) {\n\t\t\t\toutputBuf += decoder.decode(buf);\n\t\t\t\tconst nl = outputBuf.lastIndexOf(\"\\n\");\n\t\t\t\tif (nl != -1) {\n\t\t\t\t\tconsole.log(outputBuf.substring(0, nl));\n\t\t\t\t\toutputBuf = outputBuf.substring(nl + 1);\n\t\t\t\t}\n\t\t\t\treturn buf.length;\n\t\t\t},\n\t\t\twrite(fd, buf, offset, length, position, callback) {\n\t\t\t\tif (offset !== 0 || length !== buf.length || position !== null) {\n\t\t\t\t\tcallback(enosys());\n\t\t\t\t\treturn;\n\t\t\t\t}\n\t\t\t\tconst n = this.writeSync(fd, buf);\n\t\t\t\tcallback(null, n);\n\t\t\t},\n\t\t\tchmod(path, mode, callback) { callback(enosys()); },\n\t\t\tchown(path, uid, gid, callback) { callback(enosys()); },\n\t\t\tclose(fd, callback) { callback(enosys()); },\n\t\t\tfchmod(fd, mode, callback) { callback(enosys()); },\n\t\t\tfchown(fd, uid, gid, callback) { callback(enosys()); },\n\t\t\tfstat(fd, callback) { callback(enosys()); },\n\t\t\tfsync(fd, callback) { callback(null); },\n\t\t\tftruncate(fd, length, callback) { callback(enosys()); },\n\t\t\tlchown(path, uid, gid, callback) { callback(enosys()); },\n\t\t\tlink(path, link, callback) { callback(enosys()); },\n\t\t\tlstat(path, callback) { callback(enosys()); },\n\t\t\tmkdir(path, perm, callback) { callback(enosys()); },\n\t\t\topen(path, flags, mode, callback) { callback(enosys()); },\n\t\t\tread(fd, buffer, offset, length, position, callback) { callback(enosys()); },\n\t\t\treaddir(path, callback) { callback(enosys()); },\n\t\t\treadlink(path, callback) { callback(enosys()); },\n\t\t\trename(from, to, callback) { callback(enosys()); },\n\t\t\trmdir(path, callback) { callback(enosys()); },\n\t\t\tstat(path, callback) { callback(enosys()); },\n\t\t\tsymlink(path, link, callback) { callback(enosys()); },\n\t\t\ttruncate(path, length, callback) { callback(enosys()); },\n\t\t\tunlink(path, callback) { callback(enosys()); },\n\t\t\tutimes(path, atime, mtime, callback) { callback(enosys()); },\n\t\t};\n\t}\n\n\tif (!globalThis.process) {\n\t\tglobalThis.process = {\n\t\t\tgetuid() { return -1; },\n\t\t\tgetgid() { return -1; },\n\t\t\tgeteuid() { return -1; },\n\t\t\tgetegid() { return -1; },\n\t\t\tgetgroups() { throw enosys(); },\n\t\t\tpid: -1,\n\t\t\tppid: -1,\n\t\t\tumask() { throw enosys(); },\n\t\t\tcwd() { throw enosys(); },\n\t\t\tchdir() { throw enosys(); },\n\t\t}\n\t}\n\n\tif (!globalThis.crypto) {\n\t\tthrow new Error(\"globalThis.crypto is not available, polyfill required (crypto.getRandomValues only)\");\n\t}\n\n\tif (!globalThis.performance) {\n\t\tthrow new Error(\"globalThis.performance is not available, polyfill required (performance.now only)\");\n\t}\n\n\tif (!globalThis.TextEncoder) {\n\t\tthrow new Error(\"globalThis.TextEncoder is not available, polyfill required\");\n\t}\n\n\tif (!globalThis.TextDecoder) {\n\t\tthrow new Error(\"globalThis.TextDecoder is not available, polyfill required\");\n\t}\n\n\tconst encoder = new TextEncoder(\"utf-8\");\n\tconst decoder = new TextDecoder(\"utf-8\");\n\n\tglobalThis.Go = class {\n\t\tconstructor() {\n\t\t\tthis.argv = [\"js\"];\n\t\t\tthis.env = {};\n\t\t\tthis.exit = (code) => {\n\t\t\t\tif (code !== 0) {\n\t\t\t\t\tconsole.warn(\"exit code:\", code);\n\t\t\t\t}\n\t\t\t};\n\t\t\tthis._exitPromise = new Promise((resolve) => {\n\t\t\t\tthis._resolveExitPromise = resolve;\n\t\t\t});\n\t\t\tthis._pendingEvent = null;\n\t\t\tthis._scheduledTimeouts = new Map();\n\t\t\tthis._nextCallbackTimeoutID = 1;\n\n\t\t\tconst setInt64 = (addr, v) => {\n\t\t\t\tthis.mem.setUint32(addr + 0, v, true);\n\t\t\t\tthis.mem.setUint32(addr + 4, Math.floor(v / 4294967296), true);\n\t\t\t}\n\n\t\t\tconst setInt32 = (addr, v) => {\n\t\t\t\tthis.mem.setUint32(addr + 0, v, true);\n\t\t\t}\n\n\t\t\tconst getInt64 = (addr) => {\n\t\t\t\tconst low = this.mem.getUint32(addr + 0, true);\n\t\t\t\tconst high = this.mem.getInt32(addr + 4, true);\n\t\t\t\treturn low + high * 4294967296;\n\t\t\t}\n\n\t\t\tconst loadValue = (addr) => {\n\t\t\t\tconst f = this.mem.getFloat64(addr, true);\n\t\t\t\tif (f === 0) {\n\t\t\t\t\treturn undefined;\n\t\t\t\t}\n\t\t\t\tif (!isNaN(f)) {\n\t\t\t\t\treturn f;\n\t\t\t\t}\n\n\t\t\t\tconst id = this.mem.getUint32(addr, true);\n\t\t\t\treturn this._values[id];\n\t\t\t}\n\n\t\t\tconst storeValue = (addr, v) => {\n\t\t\t\tconst nanHead = 0x7FF80000;\n\n\t\t\t\tif (typeof v === \"number\" && v !== 0) {\n\t\t\t\t\tif (isNaN(v)) {\n\t\t\t\t\t\tthis.mem.setUint32(addr + 4, nanHead, true);\n\t\t\t\t\t\tthis.mem.setUint32(addr, 0, true);\n\t\t\t\t\t\treturn;\n\t\t\t\t\t}\n\t\t\t\t\tthis.mem.setFloat64(addr, v, true);\n\t\t\t\t\treturn;\n\t\t\t\t}\n\n\t\t\t\tif (v === undefined) {\n\t\t\t\t\tthis.mem.setFloat64(addr, 0, true);\n\t\t\t\t\treturn;\n\t\t\t\t}\n\n\t\t\t\tlet id = this._ids.get(v);\n\t\t\t\tif (id === undefined) {\n\t\t\t\t\tid = this._idPool.pop();\n\t\t\t\t\tif (id === undefined) {\n\t\t\t\t\t\tid = this._values.length;\n\t\t\t\t\t}\n\t\t\t\t\tthis._values[id] = v;\n\t\t\t\t\tthis._goRefCounts[id] = 0;\n\t\t\t\t\tthis._ids.set(v, id);\n\t\t\t\t}\n\t\t\t\tthis._goRefCounts[id]++;\n\t\t\t\tlet typeFlag = 0;\n\t\t\t\tswitch (typeof v) {\n\t\t\t\t\tcase \"object\":\n\t\t\t\t\t\tif (v !== null) {\n\t\t\t\t\t\t\ttypeFlag = 1;\n\t\t\t\t\t\t}\n\t\t\t\t\t\tbreak;\n\t\t\t\t\tcase \"string\":\n\t\t\t\t\t\ttypeFlag = 2;\n\t\t\t\t\t\tbreak;\n\t\t\t\t\tcase \"symbol\":\n\t\t\t\t\t\ttypeFlag = 3;\n\t\t\t\t\t\tbreak;\n\t\t\t\t\tcase \"function\":\n\t\t\t\t\t\ttypeFlag = 4;\n\t\t\t\t\t\tbreak;\n\t\t\t\t}\n\t\t\t\tthis.mem.setUint32(addr + 4, nanHead | typeFlag, true);\n\t\t\t\tthis.mem.setUint32(addr, id, true);\n\t\t\t}\n\n\t\t\tconst loadSlice = (addr) => {\n\t\t\t\tconst array = getInt64(addr + 0);\n\t\t\t\tconst len = getInt64(addr + 8);\n\t\t\t\treturn new Uint8Array(this._inst.exports.mem.buffer, array, len);\n\t\t\t}\n\n\t\t\tconst loadSliceOfValues = (addr) => {\n\t\t\t\tconst array = getInt64(addr + 0);\n\t\t\t\tconst len = getInt64(addr + 8);\n\t\t\t\tconst a = new Array(len);\n\t\t\t\tfor (let i = 0; i < len; i++) {\n\t\t\t\t\ta[i] = loadValue(array + i * 8);\n\t\t\t\t}\n\t\t\t\treturn a;\n\t\t\t}\n\n\t\t\tconst loadString = (addr) => {\n\t\t\t\tconst saddr = getInt64(addr + 0);\n\t\t\t\tconst len = getInt64(addr + 8);\n\t\t\t\treturn decoder.decode(new DataView(this._inst.exports.mem.buffer, saddr, len));\n\t\t\t}\n\n\t\t\tconst timeOrigin = Date.now() - performance.now();\n\t\t\tthis.importObject = {\n\t\t\t\t_gotest: {\n\t\t\t\t\tadd: (a, b) => a + b,\n\t\t\t\t},\n\t\t\t\tgojs: {\n\t\t\t\t\t// Go's SP does not change as long as no Go code is running. Some operations (e.g. calls, getters and setters)\n\t\t\t\t\t// may synchronously trigger a Go event handler. This makes Go code get executed in the middle of the imported\n\t\t\t\t\t// function. A goroutine can switch to a new stack if the current stack is too small (see morestack function).\n\t\t\t\t\t// This changes the SP, thus we have to update the SP used by the imported function.\n\n\t\t\t\t\t// func wasmExit(code int32)\n\t\t\t\t\t\"runtime.wasmExit\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst code = this.mem.getInt32(sp + 8, true);\n\t\t\t\t\t\tthis.exited = true;\n\t\t\t\t\t\tdelete this._inst;\n\t\t\t\t\t\tdelete this._values;\n\t\t\t\t\t\tdelete this._goRefCounts;\n\t\t\t\t\t\tdelete this._ids;\n\t\t\t\t\t\tdelete this._idPool;\n\t\t\t\t\t\tthis.exit(code);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func wasmWrite(fd uintptr, p unsafe.Pointer, n int32)\n\t\t\t\t\t\"runtime.wasmWrite\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst fd = getInt64(sp + 8);\n\t\t\t\t\t\tconst p = getInt64(sp + 16);\n\t\t\t\t\t\tconst n = this.mem.getInt32(sp + 24, true);\n\t\t\t\t\t\tfs.writeSync(fd, new Uint8Array(this._inst.exports.mem.buffer, p, n));\n\t\t\t\t\t},\n\n\t\t\t\t\t// func resetMemoryDataView()\n\t\t\t\t\t\"runtime.resetMemoryDataView\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tthis.mem = new DataView(this._inst.exports.mem.buffer);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func nanotime1() int64\n\t\t\t\t\t\"runtime.nanotime1\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tsetInt64(sp + 8, (timeOrigin + performance.now()) * 1000000);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func walltime() (sec int64, nsec int32)\n\t\t\t\t\t\"runtime.walltime\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst msec = (new Date).getTime();\n\t\t\t\t\t\tsetInt64(sp + 8, msec / 1000);\n\t\t\t\t\t\tthis.mem.setInt32(sp + 16, (msec % 1000) * 1000000, true);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func scheduleTimeoutEvent(delay int64) int32\n\t\t\t\t\t\"runtime.scheduleTimeoutEvent\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst id = this._nextCallbackTimeoutID;\n\t\t\t\t\t\tthis._nextCallbackTimeoutID++;\n\t\t\t\t\t\tthis._scheduledTimeouts.set(id, setTimeout(\n\t\t\t\t\t\t\t() => {\n\t\t\t\t\t\t\t\tthis._resume();\n\t\t\t\t\t\t\t\twhile (this._scheduledTimeouts.has(id)) {\n\t\t\t\t\t\t\t\t\t// for some reason Go failed to register the timeout event, log and try again\n\t\t\t\t\t\t\t\t\t// (temporary workaround for https://github.com/golang/go/issues/28975)\n\t\t\t\t\t\t\t\t\tconsole.warn(\"scheduleTimeoutEvent: missed timeout event\");\n\t\t\t\t\t\t\t\t\tthis._resume();\n\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t},\n\t\t\t\t\t\t\tgetInt64(sp + 8),\n\t\t\t\t\t\t));\n\t\t\t\t\t\tthis.mem.setInt32(sp + 16, id, true);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func clearTimeoutEvent(id int32)\n\t\t\t\t\t\"runtime.clearTimeoutEvent\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst id = this.mem.getInt32(sp + 8, true);\n\t\t\t\t\t\tclearTimeout(this._scheduledTimeouts.get(id));\n\t\t\t\t\t\tthis._scheduledTimeouts.delete(id);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func getRandomData(r []byte)\n\t\t\t\t\t\"runtime.getRandomData\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tcrypto.getRandomValues(loadSlice(sp + 8));\n\t\t\t\t\t},\n\n\t\t\t\t\t// func finalizeRef(v ref)\n\t\t\t\t\t\"syscall/js.finalizeRef\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst id = this.mem.getUint32(sp + 8, true);\n\t\t\t\t\t\tthis._goRefCounts[id]--;\n\t\t\t\t\t\tif (this._goRefCounts[id] === 0) {\n\t\t\t\t\t\t\tconst v = this._values[id];\n\t\t\t\t\t\t\tthis._values[id] = null;\n\t\t\t\t\t\t\tthis._ids.delete(v);\n\t\t\t\t\t\t\tthis._idPool.push(id);\n\t\t\t\t\t\t}\n\t\t\t\t\t},\n\n\t\t\t\t\t// func stringVal(value string) ref\n\t\t\t\t\t\"syscall/js.stringVal\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tstoreValue(sp + 24, loadString(sp + 8));\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueGet(v ref, p string) ref\n\t\t\t\t\t\"syscall/js.valueGet\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst result = Reflect.get(loadValue(sp + 8), loadString(sp + 16));\n\t\t\t\t\t\tsp = this._inst.exports.getsp() >>> 0; // see comment above\n\t\t\t\t\t\tstoreValue(sp + 32, result);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueSet(v ref, p string, x ref)\n\t\t\t\t\t\"syscall/js.valueSet\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tReflect.set(loadValue(sp + 8), loadString(sp + 16), loadValue(sp + 32));\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueDelete(v ref, p string)\n\t\t\t\t\t\"syscall/js.valueDelete\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tReflect.deleteProperty(loadValue(sp + 8), loadString(sp + 16));\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueIndex(v ref, i int) ref\n\t\t\t\t\t\"syscall/js.valueIndex\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tstoreValue(sp + 24, Reflect.get(loadValue(sp + 8), getInt64(sp + 16)));\n\t\t\t\t\t},\n\n\t\t\t\t\t// valueSetIndex(v ref, i int, x ref)\n\t\t\t\t\t\"syscall/js.valueSetIndex\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tReflect.set(loadValue(sp + 8), getInt64(sp + 16), loadValue(sp + 24));\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueCall(v ref, m string, args []ref) (ref, bool)\n\t\t\t\t\t\"syscall/js.valueCall\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\ttry {\n\t\t\t\t\t\t\tconst v = loadValue(sp + 8);\n\t\t\t\t\t\t\tconst m = Reflect.get(v, loadString(sp + 16));\n\t\t\t\t\t\t\tconst args = loadSliceOfValues(sp + 32);\n\t\t\t\t\t\t\tconst result = Reflect.apply(m, v, args);\n\t\t\t\t\t\t\tsp = this._inst.exports.getsp() >>> 0; // see comment above\n\t\t\t\t\t\t\tstoreValue(sp + 56, result);\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 64, 1);\n\t\t\t\t\t\t} catch (err) {\n\t\t\t\t\t\t\tsp = this._inst.exports.getsp() >>> 0; // see comment above\n\t\t\t\t\t\t\tstoreValue(sp + 56, err);\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 64, 0);\n\t\t\t\t\t\t}\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueInvoke(v ref, args []ref) (ref, bool)\n\t\t\t\t\t\"syscall/js.valueInvoke\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\ttry {\n\t\t\t\t\t\t\tconst v = loadValue(sp + 8);\n\t\t\t\t\t\t\tconst args = loadSliceOfValues(sp + 16);\n\t\t\t\t\t\t\tconst result = Reflect.apply(v, undefined, args);\n\t\t\t\t\t\t\tsp = this._inst.exports.getsp() >>> 0; // see comment above\n\t\t\t\t\t\t\tstoreValue(sp + 40, result);\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 1);\n\t\t\t\t\t\t} catch (err) {\n\t\t\t\t\t\t\tsp = this._inst.exports.getsp() >>> 0; // see comment above\n\t\t\t\t\t\t\tstoreValue(sp + 40, err);\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 0);\n\t\t\t\t\t\t}\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueNew(v ref, args []ref) (ref, bool)\n\t\t\t\t\t\"syscall/js.valueNew\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\ttry {\n\t\t\t\t\t\t\tconst v = loadValue(sp + 8);\n\t\t\t\t\t\t\tconst args = loadSliceOfValues(sp + 16);\n\t\t\t\t\t\t\tconst result = Reflect.construct(v, args);\n\t\t\t\t\t\t\tsp = this._inst.exports.getsp() >>> 0; // see comment above\n\t\t\t\t\t\t\tstoreValue(sp + 40, result);\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 1);\n\t\t\t\t\t\t} catch (err) {\n\t\t\t\t\t\t\tsp = this._inst.exports.getsp() >>> 0; // see comment above\n\t\t\t\t\t\t\tstoreValue(sp + 40, err);\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 0);\n\t\t\t\t\t\t}\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueLength(v ref) int\n\t\t\t\t\t\"syscall/js.valueLength\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tsetInt64(sp + 16, parseInt(loadValue(sp + 8).length));\n\t\t\t\t\t},\n\n\t\t\t\t\t// valuePrepareString(v ref) (ref, int)\n\t\t\t\t\t\"syscall/js.valuePrepareString\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst str = encoder.encode(String(loadValue(sp + 8)));\n\t\t\t\t\t\tstoreValue(sp + 16, str);\n\t\t\t\t\t\tsetInt64(sp + 24, str.length);\n\t\t\t\t\t},\n\n\t\t\t\t\t// valueLoadString(v ref, b []byte)\n\t\t\t\t\t\"syscall/js.valueLoadString\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst str = loadValue(sp + 8);\n\t\t\t\t\t\tloadSlice(sp + 16).set(str);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueInstanceOf(v ref, t ref) bool\n\t\t\t\t\t\"syscall/js.valueInstanceOf\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tthis.mem.setUint8(sp + 24, (loadValue(sp + 8) instanceof loadValue(sp + 16)) ? 1 : 0);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func copyBytesToGo(dst []byte, src ref) (int, bool)\n\t\t\t\t\t\"syscall/js.copyBytesToGo\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst dst = loadSlice(sp + 8);\n\t\t\t\t\t\tconst src = loadValue(sp + 32);\n\t\t\t\t\t\tif (!(src instanceof Uint8Array || src instanceof Uint8ClampedArray)) {\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 0);\n\t\t\t\t\t\t\treturn;\n\t\t\t\t\t\t}\n\t\t\t\t\t\tconst toCopy = src.subarray(0, dst.length);\n\t\t\t\t\t\tdst.set(toCopy);\n\t\t\t\t\t\tsetInt64(sp + 40, toCopy.length);\n\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 1);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func copyBytesToJS(dst ref, src []byte) (int, bool)\n\t\t\t\t\t\"syscall/js.copyBytesToJS\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst dst = loadValue(sp + 8);\n\t\t\t\t\t\tconst src = loadSlice(sp + 16);\n\t\t\t\t\t\tif (!(dst instanceof Uint8Array || dst instanceof Uint8ClampedArray)) {\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 0);\n\t\t\t\t\t\t\treturn;\n\t\t\t\t\t\t}\n\t\t\t\t\t\tconst toCopy = src.subarray(0, dst.length);\n\t\t\t\t\t\tdst.set(toCopy);\n\t\t\t\t\t\tsetInt64(sp + 40, toCopy.length);\n\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 1);\n\t\t\t\t\t},\n\n\t\t\t\t\t\"debug\": (value) => {\n\t\t\t\t\t\tconsole.log(value);\n\t\t\t\t\t},\n\t\t\t\t}\n\t\t\t};\n\t\t}\n\n\t\tasync run(instance) {\n\t\t\tif (!(instance instanceof WebAssembly.Instance)) {\n\t\t\t\tthrow new Error(\"Go.run: WebAssembly.Instance expected\");\n\t\t\t}\n\t\t\tthis._inst = instance;\n\t\t\tthis.mem = new DataView(this._inst.exports.mem.buffer);\n\t\t\tthis._values = [ // JS values that Go currently has references to, indexed by reference id\n\t\t\t\tNaN,\n\t\t\t\t0,\n\t\t\t\tnull,\n\t\t\t\ttrue,\n\t\t\t\tfalse,\n\t\t\t\tglobalThis,\n\t\t\t\tthis,\n\t\t\t];\n\t\t\tthis._goRefCounts = new Array(this._values.length).fill(Infinity); // number of references that Go has to a JS value, indexed by reference id\n\t\t\tthis._ids = new Map([ // mapping from JS values to reference ids\n\t\t\t\t[0, 1],\n\t\t\t\t[null, 2],\n\t\t\t\t[true, 3],\n\t\t\t\t[false, 4],\n\t\t\t\t[globalThis, 5],\n\t\t\t\t[this, 6],\n\t\t\t]);\n\t\t\tthis._idPool = [];   // unused ids that have been garbage collected\n\t\t\tthis.exited = false; // whether the Go program has exited\n\n\t\t\t// Pass command line arguments and environment variables to WebAssembly by writing them to the linear memory.\n\t\t\tlet offset = 4096;\n\n\t\t\tconst strPtr = (str) => {\n\t\t\t\tconst ptr = offset;\n\t\t\t\tconst bytes = encoder.encode(str + \"\\0\");\n\t\t\t\tnew Uint8Array(this.mem.buffer, offset, bytes.length).set(bytes);\n\t\t\t\toffset += bytes.length;\n\t\t\t\tif (offset % 8 !== 0) {\n\t\t\t\t\toffset += 8 - (offset % 8);\n\t\t\t\t}\n\t\t\t\treturn ptr;\n\t\t\t};\n\n\t\t\tconst argc = this.argv.length;\n\n\t\t\tconst argvPtrs = [];\n\t\t\tthis.argv.forEach((arg) => {\n\t\t\t\targvPtrs.push(strPtr(arg));\n\t\t\t});\n\t\t\targvPtrs.push(0);\n\n\t\t\tconst keys = Object.keys(this.env).sort();\n\t\t\tkeys.forEach((key) => {\n\t\t\t\targvPtrs.push(strPtr(`${key}=${this.env[key]}`));\n\t\t\t});\n\t\t\targvPtrs.push(0);\n\n\t\t\tconst argv = offset;\n\t\t\targvPtrs.forEach((ptr) => {\n\t\t\t\tthis.mem.setUint32(offset, ptr, true);\n\t\t\t\tthis.mem.setUint32(offset + 4, 0, true);\n\t\t\t\toffset += 8;\n\t\t\t});\n\n\t\t\t// The linker guarantees global data starts from at least wasmMinDataAddr.\n\t\t\t// Keep in sync with cmd/link/internal/ld/data.go:wasmMinDataAddr.\n\t\t\tconst wasmMinDataAddr = 4096 + 8192;\n\t\t\tif (offset >= wasmMinDataAddr) {\n\t\t\t\tthrow new Error(\"total length of command line and environment variables exceeds limit\");\n\t\t\t}\n\n\t\t\tthis._inst.exports.run(argc, argv);\n\t\t\tif (this.exited) {\n\t\t\t\tthis._resolveExitPromise();\n\t\t\t}\n\t\t\tawait this._exitPromise;\n\t\t}\n\n\t\t_resume() {\n\t\t\tif (this.exited) {\n\t\t\t\tthrow new Error(\"Go program has already exited\");\n\t\t\t}\n\t\t\tthis._inst.exports.resume();\n\t\t\tif (this.exited) {\n\t\t\t\tthis._resolveExitPromise();\n\t\t\t}\n\t\t}\n\n\t\t_makeFuncWrapper(id) {\n\t\t\tconst go = this;\n\t\t\treturn function () {\n\t\t\t\tconst event = { id: id, this: this, args: arguments };\n\t\t\t\tgo._pendingEvent = event;\n\t\t\t\tgo._resume();\n\t\t\t\treturn event.result;\n\t\t\t};\n\t\t}\n\t}\n})();\n"

	wasmExecJSGo120 = "// Copyright 2018 The Go Authors. All rights reserved.\n// Use of this source code is governed by a BSD-style\n// license that can be found in the LICENSE file.\n\n\"use strict\";\n\n(() => {\n\tconst enosys = () => {\n\t\tconst err = new Error(\"not implemented\");\n\t\terr.code = \"ENOSYS\";\n\t\treturn err;\n\t};\n\n\tif (!globalThis.fs) {\n\t\tlet outputBuf = \"\";\n\t\tglobalThis.fs = {\n\t\t\tconstants: { O_WRONLY: -1, O_RDWR: -1, O_CREAT: -1, O_TRUNC: -1, O_APPEND: -1, O_EXCL: -1 }, // unused\n\t\t\twriteSync(fd, buf) {\n\t\t\t\toutputBuf += decoder.decode(buf);\n\t\t\t\tconst nl = outputBuf.lastIndexOf(\"\\n\");\n\t\t\t\tif (nl != -1) {\n\t\t\t\t\tconsole.log(outputBuf.substring(0, nl));\n\t\t\t\t\toutputBuf = outputBuf.substring(nl + 1);\n\t\t\t\t}\n\t\t\t\treturn buf.length;\n\t\t\t},\n\t\t\twrite(fd, buf, offset, length, position, callback) {\n\t\t\t\tif (offset !== 0 || length !== buf.length || position !== null) {\n\t\t\t\t\tcallback(enosys());\n\t\t\t\t\treturn;\n\t\t\t\t}\n\t\t\t\tconst n = this.writeSync(fd, buf);\n\t\t\t\tcallback(null, n);\n\t\t\t},\n\t\t\tchmod(path, mode, callback) { callback(enosys()); },\n\t\t\tchown(path, uid, gid, callback) { callback(enosys()); },\n\t\t\tclose(fd, callback) { callback(enosys()); },\n\t\t\tfchmod(fd, mode, callback) { callback(enosys()); },\n\t\t\tfchown(fd, uid, gid, callback) { callback(enosys()); },\n\t\t\tfstat(fd, callback) { callback(enosys()); },\n\t\t\tfsync(fd, callback) { callback(null); },\n\t\t\tftruncate(fd, length, callback) { callback(enosys()); },\n\t\t\tlchown(path, uid, gid, callback) { callback(enosys()); },\n\t\t\tlink(path, link, callback) { callback(enosys()); },\n\t\t\tlstat(path, callback) { callback(enosys()); },\n\t\t\tmkdir(path, perm, callback) { callback(enosys()); },\n\t\t\topen(path, flags, mode, callback) { callback(enosys()); },\n\t\t\tread(fd, buffer, offset, length, position, callback) { callback(enosys()); },\n\t\t\treaddir(path, callback) { callback(enosys()); },\n\t\t\treadlink(path, callback) { callback(enosys()); },\n\t\t\trename(from, to, callback) { callback(enosys()); },\n\t\t\trmdir(path, callback) { callback(enosys()); },\n\t\t\tstat(path, callback) { callback(enosys()); },\n\t\t\tsymlink(path, link, callback) { callback(enosys()); },\n\t\t\ttruncate(path, length, callback) { callback(enosys()); },\n\t\t\tunlink(path, callback) { callback(enosys()); },\n\t\t\tutimes(path, atime, mtime, callback) { callback(enosys()); },\n\t\t};\n\t}\n\n\tif (!globalThis.process) {\n\t\tglobalThis.process = {\n\t\t\tgetuid() { return -1; },\n\t\t\tgetgid() { return -1; },\n\t\t\tgeteuid() { return -1; },\n\t\t\tgetegid() { return -1; },\n\t\t\tgetgroups() { throw enosys(); },\n\t\t\tpid: -1,\n\t\t\tppid: -1,\n\t\t\tumask() { throw enosys(); },\n\t\t\tcwd() { throw enosys(); },\n\t\t\tchdir() { throw enosys(); },\n\t\t}\n\t}\n\n\tif (!globalThis.crypto) {\n\t\tthrow new Error(\"globalThis.crypto is not available, polyfill required (crypto.getRandomValues only)\");\n\t}\n\n\tif (!globalThis.performance) {\n\t\tthrow new Error(\"globalThis.performance is not available, polyfill required (performance.now only)\");\n\t}\n\n\tif (!globalThis.TextEncoder) {\n\t\tthrow new Error(\"globalThis.TextEncoder is not available, polyfill required\");\n\t}\n\n\tif (!globalThis.TextDecoder) {\n\t\tthrow new Error(\"globalThis.TextDecoder is not available, polyfill required\");\n\t}\n\n\tconst encoder = new TextEncoder(\"utf-8\");\n\tconst decoder = new TextDecoder(\"utf-8\");\n\n\tglobalThis.Go = class {\n\t\tconstructor() {\n\t\t\tthis.argv = [\"js\"];\n\t\t\tthis.env = {};\n\t\t\tthis.exit = (code) => {\n\t\t\t\tif (code !== 0) {\n\t\t\t\t\tconsole.warn(\"exit code:\", code);\n\t\t\t\t}\n\t\t\t};\n\t\t\tthis._exitPromise = new Promise((resolve) => {\n\t\t\t\tthis._resolveExitPromise = resolve;\n\t\t\t});\n\t\t\tthis._pendingEvent = null;\n\t\t\tthis._scheduledTimeouts = new Map();\n\t\t\tthis._nextCallbackTimeoutID = 1;\n\n\t\t\tconst setInt64 = (addr, v) => {\n\t\t\t\tthis.mem.setUint32(addr + 0, v, true);\n\t\t\t\tthis.mem.setUint32(addr + 4, Math.floor(v / 4294967296), true);\n\t\t\t}\n\n\t\t\tconst getInt64 = (addr) => {\n\t\t\t\tconst low = this.mem.getUint32(addr + 0, true);\n\t\t\t\tconst high = this.mem.getInt32(addr + 4, true);\n\t\t\t\treturn low + high * 4294967296;\n\t\t\t}\n\n\t\t\tconst loadValue = (addr) => {\n\t\t\t\tconst f = this.mem.getFloat64(addr, true);\n\t\t\t\tif (f === 0) {\n\t\t\t\t\treturn undefined;\n\t\t\t\t}\n\t\t\t\tif (!isNaN(f)) {\n\t\t\t\t\treturn f;\n\t\t\t\t}\n\n\t\t\t\tconst id = this.mem.getUint32(addr, true);\n\t\t\t\treturn this._values[id];\n\t\t\t}\n\n\t\t\tconst storeValue = (addr, v) => {\n\t\t\t\tconst nanHead = 0x7FF80000;\n\n\t\t\t\tif (typeof v === \"number\" && v !== 0) {\n\t\t\t\t\tif (isNaN(v)) {\n\t\t\t\t\t\tthis.mem.setUint32(addr + 4, nanHead, true);\n\t\t\t\t\t\tthis.mem.setUint32(addr, 0, true);\n\t\t\t\t\t\treturn;\n\t\t\t\t\t}\n\t\t\t\t\tthis.mem.setFloat64(addr, v, true);\n\t\t\t\t\treturn;\n\t\t\t\t}\n\n\t\t\t\tif (v === undefined) {\n\t\t\t\t\tthis.mem.setFloat64(addr, 0, true);\n\t\t\t\t\treturn;\n\t\t\t\t}\n\n\t\t\t\tlet id = this._ids.get(v);\n\t\t\t\tif (id === undefined) {\n\t\t\t\t\tid = this._idPool.pop();\n\t\t\t\t\tif (id === undefined) {\n\t\t\t\t\t\tid = this._values.length;\n\t\t\t\t\t}\n\t\t\t\t\tthis._values[id] = v;\n\t\t\t\t\tthis._goRefCounts[id] = 0;\n\t\t\t\t\tthis._ids.set(v, id);\n\t\t\t\t}\n\t\t\t\tthis._goRefCounts[id]++;\n\t\t\t\tlet typeFlag = 0;\n\t\t\t\tswitch (typeof v) {\n\t\t\t\t\tcase \"object\":\n\t\t\t\t\t\tif (v !== null) {\n\t\t\t\t\t\t\ttypeFlag = 1;\n\t\t\t\t\t\t}\n\t\t\t\t\t\tbreak;\n\t\t\t\t\tcase \"string\":\n\t\t\t\t\t\ttypeFlag = 2;\n\t\t\t\t\t\tbreak;\n\t\t\t\t\tcase \"symbol\":\n\t\t\t\t\t\ttypeFlag = 3;\n\t\t\t\t\t\tbreak;\n\t\t\t\t\tcase \"function\":\n\t\t\t\t\t\ttypeFlag = 4;\n\t\t\t\t\t\tbreak;\n\t\t\t\t}\n\t\t\t\tthis.mem.setUint32(addr + 4, nanHead | typeFlag, true);\n\t\t\t\tthis.mem.setUint32(addr, id, true);\n\t\t\t}\n\n\t\t\tconst loadSlice = (addr) => {\n\t\t\t\tconst array = getInt64(addr + 0);\n\t\t\t\tconst len = getInt64(addr + 8);\n\t\t\t\treturn new Uint8Array(this._inst.exports.mem.buffer, array, len);\n\t\t\t}\n\n\t\t\tconst loadSliceOfValues = (addr) => {\n\t\t\t\tconst array = getInt64(addr + 0);\n\t\t\t\tconst len = getInt64(addr + 8);\n\t\t\t\tconst a = new Array(len);\n\t\t\t\tfor (let i = 0; i < len; i++) {\n\t\t\t\t\ta[i] = loadValue(array + i * 8);\n\t\t\t\t}\n\t\t\t\treturn a;\n\t\t\t}\n\n\t\t\tconst loadString = (addr) => {\n\t\t\t\tconst saddr = getInt64(addr + 0);\n\t\t\t\tconst len = getInt64(addr + 8);\n\t\t\t\treturn decoder.decode(new DataView(this._inst.exports.mem.buffer, saddr, len));\n\t\t\t}\n\n\t\t\tconst timeOrigin = Date.now() - performance.now();\n\t\t\tthis.importObject = {\n\t\t\t\tgo: {\n\t\t\t\t\t// Go's SP does not change as long as no Go code is running. Some operations (e.g. calls, getters and setters)\n\t\t\t\t\t// may synchronously trigger a Go event handler. This makes Go code get executed in the middle of the imported\n\t\t\t\t\t// function. A goroutine can switch to a new stack if the current stack is too small (see morestack function).\n\t\t\t\t\t// This changes the SP, thus we have to update the SP used by the imported function.\n\n\t\t\t\t\t// func wasmExit(code int32)\n\t\t\t\t\t\"runtime.wasmExit\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst code = this.mem.getInt32(sp + 8, true);\n\t\t\t\t\t\tthis.exited = true;\n\t\t\t\t\t\tdelete this._inst;\n\t\t\t\t\t\tdelete this._values;\n\t\t\t\t\t\tdelete this._goRefCounts;\n\t\t\t\t\t\tdelete this._ids;\n\t\t\t\t\t\tdelete this._idPool;\n\t\t\t\t\t\tthis.exit(code);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func wasmWrite(fd uintptr, p unsafe.Pointer, n int32)\n\t\t\t\t\t\"runtime.wasmWrite\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst fd = getInt64(sp + 8);\n\t\t\t\t\t\tconst p = getInt64(sp + 16);\n\t\t\t\t\t\tconst n = this.mem.getInt32(sp + 24, true);\n\t\t\t\t\t\tfs.writeSync(fd, new Uint8Array(this._inst.exports.mem.buffer, p, n));\n\t\t\t\t\t},\n\n\t\t\t\t\t// func resetMemoryDataView()\n\t\t\t\t\t\"runtime.resetMemoryDataView\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tthis.mem = new DataView(this._inst.exports.mem.buffer);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func nanotime1() int64\n\t\t\t\t\t\"runtime.nanotime1\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tsetInt64(sp + 8, (timeOrigin + performance.now()) * 1000000);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func walltime() (sec int64, nsec int32)\n\t\t\t\t\t\"runtime.walltime\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst msec = (new Date).getTime();\n\t\t\t\t\t\tsetInt64(sp + 8, msec / 1000);\n\t\t\t\t\t\tthis.mem.setInt32(sp + 16, (msec % 1000) * 1000000, true);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func scheduleTimeoutEvent(delay int64) int32\n\t\t\t\t\t\"runtime.scheduleTimeoutEvent\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst id = this._nextCallbackTimeoutID;\n\t\t\t\t\t\tthis._nextCallbackTimeoutID++;\n\t\t\t\t\t\tthis._scheduledTimeouts.set(id, setTimeout(\n\t\t\t\t\t\t\t() => {\n\t\t\t\t\t\t\t\tthis._resume();\n\t\t\t\t\t\t\t\twhile (this._scheduledTimeouts.has(id)) {\n\t\t\t\t\t\t\t\t\t// for some reason Go failed to register the timeout event, log and try again\n\t\t\t\t\t\t\t\t\t// (temporary workaround for https://github.com/golang/go/issues/28975)\n\t\t\t\t\t\t\t\t\tconsole.warn(\"scheduleTimeoutEvent: missed timeout event\");\n\t\t\t\t\t\t\t\t\tthis._resume();\n\t\t\t\t\t\t\t\t}\n\t\t\t\t\t\t\t},\n\t\t\t\t\t\t\tgetInt64(sp + 8) + 1, // setTimeout has been seen to fire up to 1 millisecond early\n\t\t\t\t\t\t));\n\t\t\t\t\t\tthis.mem.setInt32(sp + 16, id, true);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func clearTimeoutEvent(id int32)\n\t\t\t\t\t\"runtime.clearTimeoutEvent\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst id = this.mem.getInt32(sp + 8, true);\n\t\t\t\t\t\tclearTimeout(this._scheduledTimeouts.get(id));\n\t\t\t\t\t\tthis._scheduledTimeouts.delete(id);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func getRandomData(r []byte)\n\t\t\t\t\t\"runtime.getRandomData\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tcrypto.getRandomValues(loadSlice(sp + 8));\n\t\t\t\t\t},\n\n\t\t\t\t\t// func finalizeRef(v ref)\n\t\t\t\t\t\"syscall/js.finalizeRef\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst id = this.mem.getUint32(sp + 8, true);\n\t\t\t\t\t\tthis._goRefCounts[id]--;\n\t\t\t\t\t\tif (this._goRefCounts[id] === 0) {\n\t\t\t\t\t\t\tconst v = this._values[id];\n\t\t\t\t\t\t\tthis._values[id] = null;\n\t\t\t\t\t\t\tthis._ids.delete(v);\n\t\t\t\t\t\t\tthis._idPool.push(id);\n\t\t\t\t\t\t}\n\t\t\t\t\t},\n\n\t\t\t\t\t// func stringVal(value string) ref\n\t\t\t\t\t\"syscall/js.stringVal\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tstoreValue(sp + 24, loadString(sp + 8));\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueGet(v ref, p string) ref\n\t\t\t\t\t\"syscall/js.valueGet\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst result = Reflect.get(loadValue(sp + 8), loadString(sp + 16));\n\t\t\t\t\t\tsp = this._inst.exports.getsp() >>> 0; // see comment above\n\t\t\t\t\t\tstoreValue(sp + 32, result);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueSet(v ref, p string, x ref)\n\t\t\t\t\t\"syscall/js.valueSet\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tReflect.set(loadValue(sp + 8), loadString(sp + 16), loadValue(sp + 32));\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueDelete(v ref, p string)\n\t\t\t\t\t\"syscall/js.valueDelete\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tReflect.deleteProperty(loadValue(sp + 8), loadString(sp + 16));\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueIndex(v ref, i int) ref\n\t\t\t\t\t\"syscall/js.valueIndex\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tstoreValue(sp + 24, Reflect.get(loadValue(sp + 8), getInt64(sp + 16)));\n\t\t\t\t\t},\n\n\t\t\t\t\t// valueSetIndex(v ref, i int, x ref)\n\t\t\t\t\t\"syscall/js.valueSetIndex\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tReflect.set(loadValue(sp + 8), getInt64(sp + 16), loadValue(sp + 24));\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueCall(v ref, m string, args []ref) (ref, bool)\n\t\t\t\t\t\"syscall/js.valueCall\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\ttry {\n\t\t\t\t\t\t\tconst v = loadValue(sp + 8);\n\t\t\t\t\t\t\tconst m = Reflect.get(v, loadString(sp + 16));\n\t\t\t\t\t\t\tconst args = loadSliceOfValues(sp + 32);\n\t\t\t\t\t\t\tconst result = Reflect.apply(m, v, args);\n\t\t\t\t\t\t\tsp = this._inst.exports.getsp() >>> 0; // see comment above\n\t\t\t\t\t\t\tstoreValue(sp + 56, result);\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 64, 1);\n\t\t\t\t\t\t} catch (err) {\n\t\t\t\t\t\t\tsp = this._inst.exports.getsp() >>> 0; // see comment above\n\t\t\t\t\t\t\tstoreValue(sp + 56, err);\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 64, 0);\n\t\t\t\t\t\t}\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueInvoke(v ref, args []ref) (ref, bool)\n\t\t\t\t\t\"syscall/js.valueInvoke\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\ttry {\n\t\t\t\t\t\t\tconst v = loadValue(sp + 8);\n\t\t\t\t\t\t\tconst args = loadSliceOfValues(sp + 16);\n\t\t\t\t\t\t\tconst result = Reflect.apply(v, undefined, args);\n\t\t\t\t\t\t\tsp = this._inst.exports.getsp() >>> 0; // see comment above\n\t\t\t\t\t\t\tstoreValue(sp + 40, result);\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 1);\n\t\t\t\t\t\t} catch (err) {\n\t\t\t\t\t\t\tsp = this._inst.exports.getsp() >>> 0; // see comment above\n\t\t\t\t\t\t\tstoreValue(sp + 40, err);\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 0);\n\t\t\t\t\t\t}\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueNew(v ref, args []ref) (ref, bool)\n\t\t\t\t\t\"syscall/js.valueNew\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\ttry {\n\t\t\t\t\t\t\tconst v = loadValue(sp + 8);\n\t\t\t\t\t\t\tconst args = loadSliceOfValues(sp + 16);\n\t\t\t\t\t\t\tconst result = Reflect.construct(v, args);\n\t\t\t\t\t\t\tsp = this._inst.exports.getsp() >>> 0; // see comment above\n\t\t\t\t\t\t\tstoreValue(sp + 40, result);\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 1);\n\t\t\t\t\t\t} catch (err) {\n\t\t\t\t\t\t\tsp = this._inst.exports.getsp() >>> 0; // see comment above\n\t\t\t\t\t\t\tstoreValue(sp + 40, err);\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 0);\n\t\t\t\t\t\t}\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueLength(v ref) int\n\t\t\t\t\t\"syscall/js.valueLength\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tsetInt64(sp + 16, parseInt(loadValue(sp + 8).length));\n\t\t\t\t\t},\n\n\t\t\t\t\t// valuePrepareString(v ref) (ref, int)\n\t\t\t\t\t\"syscall/js.valuePrepareString\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst str = encoder.encode(String(loadValue(sp + 8)));\n\t\t\t\t\t\tstoreValue(sp + 16, str);\n\t\t\t\t\t\tsetInt64(sp + 24, str.length);\n\t\t\t\t\t},\n\n\t\t\t\t\t// valueLoadString(v ref, b []byte)\n\t\t\t\t\t\"syscall/js.valueLoadString\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst str = loadValue(sp + 8);\n\t\t\t\t\t\tloadSlice(sp + 16).set(str);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func valueInstanceOf(v ref, t ref) bool\n\t\t\t\t\t\"syscall/js.valueInstanceOf\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tthis.mem.setUint8(sp + 24, (loadValue(sp + 8) instanceof loadValue(sp + 16)) ? 1 : 0);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func copyBytesToGo(dst []byte, src ref) (int, bool)\n\t\t\t\t\t\"syscall/js.copyBytesToGo\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst dst = loadSlice(sp + 8);\n\t\t\t\t\t\tconst src = loadValue(sp + 32);\n\t\t\t\t\t\tif (!(src instanceof Uint8Array || src instanceof Uint8ClampedArray)) {\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 0);\n\t\t\t\t\t\t\treturn;\n\t\t\t\t\t\t}\n\t\t\t\t\t\tconst toCopy = src.subarray(0, dst.length);\n\t\t\t\t\t\tdst.set(toCopy);\n\t\t\t\t\t\tsetInt64(sp + 40, toCopy.length);\n\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 1);\n\t\t\t\t\t},\n\n\t\t\t\t\t// func copyBytesToJS(dst ref, src []byte) (int, bool)\n\t\t\t\t\t\"syscall/js.copyBytesToJS\": (sp) => {\n\t\t\t\t\t\tsp >>>= 0;\n\t\t\t\t\t\tconst dst = loadValue(sp + 8);\n\t\t\t\t\t\tconst src = loadSlice(sp + 16);\n\t\t\t\t\t\tif (!(dst instanceof Uint8Array || dst instanceof Uint8ClampedArray)) {\n\t\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 0);\n\t\t\t\t\t\t\treturn;\n\t\t\t\t\t\t}\n\t\t\t\t\t\tconst toCopy = src.subarray(0, dst.length);\n\t\t\t\t\t\tdst.set(toCopy);\n\t\t\t\t\t\tsetInt64(sp + 40, toCopy.length);\n\t\t\t\t\t\tthis.mem.setUint8(sp + 48, 1);\n\t\t\t\t\t},\n\n\t\t\t\t\t\"debug\": (value) => {\n\t\t\t\t\t\tconsole.log(value);\n\t\t\t\t\t},\n\t\t\t\t}\n\t\t\t};\n\t\t}\n\n\t\tasync run(instance) {\n\t\t\tif (!(instance instanceof WebAssembly.Instance)) {\n\t\t\t\tthrow new Error(\"Go.run: WebAssembly.Instance expected\");\n\t\t\t}\n\t\t\tthis._inst = instance;\n\t\t\tthis.mem = new DataView(this._inst.exports.mem.buffer);\n\t\t\tthis._values = [ // JS values that Go currently has references to, indexed by reference id\n\t\t\t\tNaN,\n\t\t\t\t0,\n\t\t\t\tnull,\n\t\t\t\ttrue,\n\t\t\t\tfalse,\n\t\t\t\tglobalThis,\n\t\t\t\tthis,\n\t\t\t];\n\t\t\tthis._goRefCounts = new Array(this._values.length).fill(Infinity); // number of references that Go has to a JS value, indexed by reference id\n\t\t\tthis._ids = new Map([ // mapping from JS values to reference ids\n\t\t\t\t[0, 1],\n\t\t\t\t[null, 2],\n\t\t\t\t[true, 3],\n\t\t\t\t[false, 4],\n\t\t\t\t[globalThis, 5],\n\t\t\t\t[this, 6],\n\t\t\t]);\n\t\t\tthis._idPool = [];   // unused ids that have been garbage collected\n\t\t\tthis.exited = false; // whether the Go program has exited\n\n\t\t\t// Pass command line arguments and environment variables to WebAssembly by writing them to the linear memory.\n\t\t\tlet offset = 4096;\n\n\t\t\tconst strPtr = (str) => {\n\t\t\t\tconst ptr = offset;\n\t\t\t\tconst bytes = encoder.encode(str + \"\\0\");\n\t\t\t\tnew Uint8Array(this.mem.buffer, offset, bytes.length).set(bytes);\n\t\t\t\toffset += bytes.length;\n\t\t\t\tif (offset % 8 !== 0) {\n\t\t\t\t\toffset += 8 - (offset % 8);\n\t\t\t\t}\n\t\t\t\treturn ptr;\n\t\t\t};\n\n\t\t\tconst argc = this.argv.length;\n\n\t\t\tconst argvPtrs = [];\n\t\t\tthis.argv.forEach((arg) => {\n\t\t\t\targvPtrs.push(strPtr(arg));\n\t\t\t});\n\t\t\targvPtrs.push(0);\n\n\t\t\tconst keys = Object.keys(this.env).sort();\n\t\t\tkeys.forEach((key) => {\n\t\t\t\targvPtrs.push(strPtr(`${key}=${this.env[key]}`));\n\t\t\t});\n\t\t\targvPtrs.push(0);\n\n\t\t\tconst argv = offset;\n\t\t\targvPtrs.forEach((ptr) => {\n\t\t\t\tthis.mem.setUint32(offset, ptr, true);\n\t\t\t\tthis.mem.setUint32(offset + 4, 0, true);\n\t\t\t\toffset += 8;\n\t\t\t});\n\n\t\t\t// The linker guarantees global data starts from at least wasmMinDataAddr.\n\t\t\t// Keep in sync with cmd/link/internal/ld/data.go:wasmMinDataAddr.\n\t\t\tconst wasmMinDataAddr = 4096 + 8192;\n\t\t\tif (offset >= wasmMinDataAddr) {\n\t\t\t\tthrow new Error(\"total length of command line and environment variables exceeds limit\");\n\t\t\t}\n\n\t\t\tthis._inst.exports.run(argc, argv);\n\t\t\tif (this.exited) {\n\t\t\t\tthis._resolveExitPromise();\n\t\t\t}\n\t\t\tawait this._exitPromise;\n\t\t}\n\n\t\t_resume() {\n\t\t\tif (this.exited) {\n\t\t\t\tthrow new Error(\"Go program has already exited\");\n\t\t\t}\n\t\t\tthis._inst.exports.resume();\n\t\t\tif (this.exited) {\n\t\t\t\tthis._resolveExitPromise();\n\t\t\t}\n\t\t}\n\n\t\t_makeFuncWrapper(id) {\n\t\t\tconst go = this;\n\t\t\treturn function () {\n\t\t\t\tconst event = { id: id, this: this, args: arguments };\n\t\t\t\tgo._pendingEvent = event;\n\t\t\t\tgo._resume();\n\t\t\t\treturn event.result;\n\t\t\t};\n\t\t}\n\t}\n})();\n"
)
