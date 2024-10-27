#include "textflag.h"
                       
TEXT ·Write(SB),NOSPLIT,$0
	DMB $0xa    // ISHST: store-store barrier
	RET
                                                                             
TEXT ·Read(SB),NOSPLIT,$0
	DMB $0x9    // ISHLD: load-load barrier
	RET

TEXT ·Full(SB),NOSPLIT,$0
	DMB $0xb    // ISHLD: load-load barrier
	RET
	
TEXT ·LoadUint16(SB),NOSPLIT,$0-10
	MOVD  ptr+0(FP), R0
	MOVHU (R0), R1
	MOVD  R1, ret+8(FP)
	RET
                                                                                                                                        
	
