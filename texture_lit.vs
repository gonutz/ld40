float4x4 mvp : register(c0);

struct input {
	float4 position: POSITION0;
	float3 normal  : NORMAL0;
	float2 texCoord: TEXCOORD0;
};

struct output {
	float4 position: POSITION0;
	float4 color   : COLOR0;
	float2 texCoord: TEXCOORD0;
};

void main(in input IN, out output OUT) {
	float3 diffuseDir = normalize(-float3(-0.7, -0.1, 0.7));
	float4 diffuseColor = float4(1, 1, 1, 1);
	float diffusePower = 1.0;
	
	OUT.position = mul(IN.position, mvp);
	// TODO if we have a model transform it must be applied to the normal as well
	float lightPower = dot(IN.normal, diffuseDir);
	OUT.color = saturate(diffuseColor * diffusePower * lightPower);
	OUT.texCoord = IN.texCoord;
}
