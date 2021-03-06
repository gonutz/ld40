float4x4 mvp : register(c0);

struct input {
	float4 position : POSITION;
};

struct output {
	float4 position : POSITION;
};

void main(in input IN, out output OUT) {
	OUT.position = mul(IN.position, mvp);
}
