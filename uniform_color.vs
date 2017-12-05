float4x4 mvp : register(c0);
float4 color : register(c4);

struct input {
	float4 position : POSITION;
};

struct output {
	float4 position : POSITION;
	float4 color : COLOR0;
};

void main(in input IN, out output OUT) {
	OUT.position = mul(IN.position, mvp);
	OUT.color = color;
}
