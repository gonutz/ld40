float4x4 mvp : register(c0);

struct input {
	float4 position : POSITION;
	float2 texCoord: TEXCOORD0;
};

struct output {
	float4 position : POSITION;
	float2 texCoord: TEXCOORD0;
};

void main(in input IN, out output OUT) {
	OUT.position = mul(IN.position, mvp);
	OUT.texCoord = IN.texCoord;
}
