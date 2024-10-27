#include "textflag.h"

TEXT ·Write(SB),NOSPLIT,$0
	RET         // x86 TSO: stores not reordered past stores

TEXT ·Read(SB),NOSPLIT,$0
	RET         // x86 TSO: loads not reordered past loads

TEXT ·Full(SB),NOSPLIT,$0
      MFENCE      // x86 TSO allows store-load reordering; MFENCE prevents it
      RET


TEXT ·LoadUint16(SB),NOSPLIT,$0-10
      MOVQ    ptr+0(FP), AX
      MOVWLZX (AX), CX
      MOVW    CX, ret+8(FP)
      RET

