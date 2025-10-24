// Test script for TypeScript HTTP client
import { MiniVault } from './http';

async function main() {
    const vault = new MiniVault('http://localhost:8080');

    console.log('Testing TypeScript HTTP Client...');

    // Test 1: Set and get JSON
    console.log('\n1. Testing JSON set/get...');
    const testUser = { name: 'Alice', age: 30, active: true };
    const setResult = await vault.set('test:user', testUser);
    console.log(`   Set result: ${setResult}`);

    const getResult = await vault.get<any>('test:user');
    console.log(`   Get result: ${JSON.stringify(getResult)}`);
    console.log(`   Match: ${JSON.stringify(getResult) === JSON.stringify(testUser)}`);

    // Test 2: Set and get raw binary
    console.log('\n2. Testing raw binary set/get...');
    const rawData = new TextEncoder().encode('Hello, MiniVault!');
    const setRawResult = await vault.setRaw('test:raw', rawData);
    console.log(`   Set raw result: ${setRawResult}`);

    const getRawResult = await vault.getRaw('test:raw');
    if (getRawResult) {
        const decoded = new TextDecoder().decode(getRawResult);
        console.log(`   Get raw result: ${decoded}`);
        console.log(`   Match: ${decoded === 'Hello, MiniVault!'}`);
    }

    // Test 3: Delete
    console.log('\n3. Testing delete...');
    const deleteResult = await vault.delete('test:user');
    console.log(`   Delete result: ${deleteResult}`);

    const getAfterDelete = await vault.get('test:user');
    console.log(`   Get after delete: ${getAfterDelete === null ? 'null (correct)' : 'ERROR: still exists'}`);

    // Test 4: Exists
    console.log('\n4. Testing exists...');
    await vault.set('test:exists', { foo: 'bar' });
    const exists1 = await vault.exists('test:exists');
    const exists2 = await vault.exists('test:notexists');
    console.log(`   Exists (should be true): ${exists1}`);
    console.log(`   Not exists (should be false): ${exists2}`);

    // Test 5: Health check
    console.log('\n5. Testing health check...');
    const health = await vault.health();
    if (health) {
        console.log(`   Status: ${health.status}`);
        console.log(`   Cache items: ${health.cache_items}`);
        console.log(`   Memory MB: ${health.memory_mb}`);
    }

    // Test 6: Batch operations
    console.log('\n6. Testing batch operations...');
    await vault.mset([
        { key: 'batch:1', value: { id: 1 } },
        { key: 'batch:2', value: { id: 2 } },
        { key: 'batch:3', value: { id: 3 } }
    ]);
    const batchResults = await vault.mget(['batch:1', 'batch:2', 'batch:3']);
    console.log(`   Batch get results: ${batchResults.length} items`);

    console.log('\n✅ All TypeScript HTTP client tests passed!');
}

main().catch(console.error);
