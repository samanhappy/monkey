package monkey

import (
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/arch/x86/x86asm"
)

// Assembles a jump to a function value
func jmpToFunctionValue(double uintptr) []byte {
	res := make([]byte, 0, 24)
	d0d1 := double & 0xFFFF
	d2d3 := double >> 16 & 0xFFFF
	d4d5 := double >> 32 & 0xFFFF
	d6d7 := double >> 48 & 0xFFFF

	res = append(res, movImm(0b10, 0, d0d1)...)          // MOVZ x26, double[16:0]
	res = append(res, movImm(0b11, 1, d2d3)...)          // MOVK x26, double[32:16]
	res = append(res, movImm(0b11, 2, d4d5)...)          // MOVK x26, double[48:32]
	res = append(res, movImm(0b11, 3, d6d7)...)          // MOVK x26, double[64:48]
	res = append(res, []byte{0x4A, 0x03, 0x40, 0xF9}...) // LDR x10, [x26]
	res = append(res, []byte{0x40, 0x01, 0x1F, 0xD6}...) // BR x10

	return res
}

func movImm(opc, shift int, val uintptr) []byte {
	var m uint32 = 26          // rd
	m |= uint32(val) << 5      // imm16
	m |= uint32(shift&3) << 21 // hw
	m |= 0b100101 << 23        // const
	m |= uint32(opc&0x3) << 29 // opc
	m |= 0b1 << 31             // sf

	res := make([]byte, 4)
	*(*uint32)(unsafe.Pointer(&res[0])) = m

	return res
}

func littleEndian(to uintptr) []byte {
	return []byte{
		byte(to),
		byte(to >> 8),
		byte(to >> 16),
		byte(to >> 24),
		byte(to >> 32),
		byte(to >> 40),
		byte(to >> 48),
		byte(to >> 56),
	}
}

// Assembles a jump to a function value
func jmpToGoFn(to uintptr) []byte {
	return []byte{
		0x48, 0xBA,
		byte(to),
		byte(to >> 8),
		byte(to >> 16),
		byte(to >> 24),
		byte(to >> 32),
		byte(to >> 40),
		byte(to >> 48),
		byte(to >> 56), // movabs rdx,to
		0xFF, 0x22,     // jmp QWORD PTR [rdx]
	}
}

func jmpTable(g, to uintptr, gofn bool) []byte {
	b := []byte{
		// movq r13, g
		0x49, 0xBD,
		byte(g),
		byte(g >> 8),
		byte(g >> 16),
		byte(g >> 24),
		byte(g >> 32),
		byte(g >> 40),
		byte(g >> 48),
		byte(g >> 56),
		// cmp r12, r13
		0x4D, 0x39, 0xEC,
		// jne $+(2+12)
		0x75, 0x0c,
	}
	if gofn {
		b = append(b, jmpToGoFn(to)...)
	} else {
		b = append(b, jmpToFunctionValue(to)...)
	}
	return b
}

func alginPatch(from uintptr) (original []byte) {
	f := rawMemoryAccess(from, 32)

	s := 0
	for {
		i, err := x86asm.Decode(f[s:], 64)
		if err != nil {
			panic(err)
		}
		original = append(original, f[s:s+i.Len]...)
		s += i.Len
		if s >= 13 {
			return
		}
	}
}

func getFirstCallFunc(from uintptr) uintptr {
	f := rawMemoryAccess(from, 1024)

	s := 0
	for {
		i, err := x86asm.Decode(f[s:], 64)
		if err != nil {
			panic(err)
		}
		if i.Op == x86asm.CALL {
			arg := i.Args[0]
			imm := arg.(x86asm.Rel)
			next := from + uintptr(s+i.Len)
			var to uintptr
			if imm > 0 {
				to = next + uintptr(imm)
			} else {
				to = next - uintptr(-imm)
			}
			f := runtime.FuncForPC(to)
			// 泛型函数的名字中包含 [...]
			if strings.Index(f.Name(), "[") > 0 {
				return to
			}
		}
		s += i.Len
		if s >= 1024 {
			panic("Can not find CALL instruction")
		}
	}
}
