// Loomarr's channel watermark on the VAAPI family (#1613): a straight-alpha blend of the bug onto
// the programme, run by FFmpeg's program_opencl on frames mapped from VAAPI (watermark.go).
//
// program_opencl calls the kernel once per plane of the NV12 output, with the same plane of every
// input, and passes no other arguments. So the bug and its alpha are full frames, the bug already
// in place (alpha 0 elsewhere), and every input is NV12-shaped:
//   - plane 0 (1 channel): frame Y, bug Y, alpha;
//   - plane 1 (2 channels, half size): frame U,V, bug U,V, and the 2x2-mean alpha in both channels.
// One componentwise mix is therefore right on both planes, and alpha 0 returns the frame exactly.
__kernel void blend_bug(__write_only image2d_t dst, unsigned int index,
                        __read_only image2d_t frame, __read_only image2d_t bug, __read_only image2d_t alpha)
{
    const sampler_t s = CLK_NORMALIZED_COORDS_FALSE | CLK_ADDRESS_CLAMP_TO_EDGE | CLK_FILTER_NEAREST;
    int2 p = (int2)(get_global_id(0), get_global_id(1));
    write_imagef(dst, p, mix(read_imagef(frame, s, p), read_imagef(bug, s, p), read_imagef(alpha, s, p)));
}
