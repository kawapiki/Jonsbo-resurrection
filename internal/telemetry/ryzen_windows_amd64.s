#include "textflag.h"

TEXT ·cpuIdentity(SB), NOSPLIT, $0-16
 MOVL $0, AX
 CPUID
 MOVL BX, vendorB+4(FP)
 MOVL DX, vendorD+8(FP)
 MOVL CX, vendorC+12(FP)
 MOVL $1, AX
 CPUID
 MOVL AX, signature+0(FP)
 RET
