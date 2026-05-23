package browser

import "fmt"

// GetAdvancedStealthJS generates a JS snippet that intercepts Canvas, WebGL, AudioContext, and Font APIs
// and adds consistent noise based on the profileID.
func GetAdvancedStealthJS(profileID string) string {
	// A simple JS script that seeds a PRNG with the profileID and overrides native methods.
	return fmt.Sprintf(`
(() => {
    const profileID = "%s";
    
    // Simple PRNG seeded by profileID
    function xmur3(str) {
        for(var i = 0, h = 1779033703 ^ str.length; i < str.length; i++) {
            h = Math.imul(h ^ str.charCodeAt(i), 3432918353);
            h = h << 13 | h >>> 19;
        }
        return function() {
            h = Math.imul(h ^ (h >>> 16), 2246822507);
            h = Math.imul(h ^ (h >>> 13), 3266489909);
            return (h ^= h >>> 16) >>> 0;
        }
    }
    const seed = xmur3(profileID)();
    
    // Generate a consistent pseudo-random float between -1 and 1
    function consistentNoise() {
        let x = Math.sin(seed++) * 10000;
        return (x - Math.floor(x)) * 2 - 1;
    }

    // 1. Canvas Spoofing (Noise to getImageData and toDataURL)
    const originalGetImageData = CanvasRenderingContext2D.prototype.getImageData;
    const originalToDataURL = HTMLCanvasElement.prototype.toDataURL;

    Object.defineProperty(CanvasRenderingContext2D.prototype, 'getImageData', {
        value: function() {
            const imageData = originalGetImageData.apply(this, arguments);
            // Apply slight noise to one of the pixels to change the hash
            if (imageData.data.length > 0) {
                const noiseIdx = Math.floor(Math.abs(consistentNoise()) * (imageData.data.length / 4)) * 4;
                if (noiseIdx < imageData.data.length) {
                    imageData.data[noiseIdx] = Math.min(255, imageData.data[noiseIdx] + (consistentNoise() > 0 ? 1 : -1));
                }
            }
            return imageData;
        }
    });

    Object.defineProperty(HTMLCanvasElement.prototype, 'toDataURL', {
        value: function() {
            const ctx = this.getContext('2d');
            if (ctx) {
                const w = this.width;
                const h = this.height;
                if (w > 0 && h > 0) {
                    const imageData = originalGetImageData.call(ctx, 0, 0, w, h);
                    const noiseIdx = Math.floor(Math.abs(consistentNoise()) * (imageData.data.length / 4)) * 4;
                    if (noiseIdx < imageData.data.length) {
                        imageData.data[noiseIdx] = Math.min(255, imageData.data[noiseIdx] + (consistentNoise() > 0 ? 1 : -1));
                    }
                    ctx.putImageData(imageData, 0, 0);
                }
            }
            return originalToDataURL.apply(this, arguments);
        }
    });

    // 2. WebGL Spoofing (Change Vendor / Renderer)
    const originalGetParameter = WebGLRenderingContext.prototype.getParameter;
    const vendors = ["Intel Inc.", "NVIDIA Corporation", "AMD"];
    const renderers = ["Intel(R) UHD Graphics 620", "NVIDIA GeForce RTX 3060", "AMD Radeon Pro 5300M"];
    
    const fakeVendor = vendors[Math.floor(Math.abs(consistentNoise()) * vendors.length)];
    const fakeRenderer = renderers[Math.floor(Math.abs(consistentNoise()) * renderers.length)];

    const hookWebGL = (proto) => {
        const orig = proto.getParameter;
        Object.defineProperty(proto, 'getParameter', {
            value: function(parameter) {
                // 37445 = UNMASKED_VENDOR_WEBGL
                if (parameter === 37445) return fakeVendor;
                // 37446 = UNMASKED_RENDERER_WEBGL
                if (parameter === 37446) return fakeRenderer;
                return orig.apply(this, arguments);
            }
        });
    };
    hookWebGL(WebGLRenderingContext.prototype);
    if (typeof WebGL2RenderingContext !== 'undefined') hookWebGL(WebGL2RenderingContext.prototype);

    // 3. AudioContext Spoofing
    const originalGetChannelData = AudioBuffer.prototype.getChannelData;
    if (originalGetChannelData) {
        Object.defineProperty(AudioBuffer.prototype, 'getChannelData', {
            value: function() {
                const results = originalGetChannelData.apply(this, arguments);
                if (results && results.length > 0) {
                    results[0] += consistentNoise() * 0.0000001; // tiny undetectable noise
                }
                return results;
            }
        });
    }

    const originalGetFloatFrequencyData = AnalyserNode.prototype.getFloatFrequencyData;
    if (originalGetFloatFrequencyData) {
        Object.defineProperty(AnalyserNode.prototype, 'getFloatFrequencyData', {
            value: function(array) {
                originalGetFloatFrequencyData.apply(this, arguments);
                if (array && array.length > 0) {
                    array[0] += consistentNoise() * 0.1;
                }
            }
        });
    }

    // 4. Font (measureText) Spoofing
    const originalMeasureText = CanvasRenderingContext2D.prototype.measureText;
    Object.defineProperty(CanvasRenderingContext2D.prototype, 'measureText', {
        value: function() {
            const metrics = originalMeasureText.apply(this, arguments);
            // Overriding properties on TextMetrics requires some trickery,
            // but we can return a Proxy to spoof the width.
            return new Proxy(metrics, {
                get(target, prop) {
                    if (prop === 'width') {
                        return target.width + (consistentNoise() * 0.01);
                    }
                    const val = target[prop];
                    return typeof val === 'function' ? val.bind(target) : val;
                }
            });
        }
    });

    // Clean up toString to look native (Best Effort)
    const hideHook = (func, name) => {
        Object.defineProperty(func, 'name', { value: name });
        Object.defineProperty(func, 'toString', {
            value: () => 'function ' + name + '() { [native code] }'
        });
    };
    hideHook(CanvasRenderingContext2D.prototype.getImageData, 'getImageData');
    hideHook(HTMLCanvasElement.prototype.toDataURL, 'toDataURL');
    hideHook(WebGLRenderingContext.prototype.getParameter, 'getParameter');
    hideHook(AudioBuffer.prototype.getChannelData, 'getChannelData');
    hideHook(AnalyserNode.prototype.getFloatFrequencyData, 'getFloatFrequencyData');
    hideHook(CanvasRenderingContext2D.prototype.measureText, 'measureText');

})();
    `, profileID)
}
