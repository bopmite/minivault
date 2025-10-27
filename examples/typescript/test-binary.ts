// Test script for TypeScript Binary client
import { MiniVaultBinary } from './binary';

async function main() {
    const client = new MiniVaultBinary('localhost:3000');

    console.log('Testing TypeScript Binary Client...');

    // Test 1: Set and get raw bytes
    console.log('\n1. Testing raw set/get...');
    const testData = Buffer.from('Hello from binary protocol!');
    const setResult = await client.set('binary:test', testData);
    console.log(`   Set result: ${setResult}`);

    const getData = await client.get('binary:test');
    if (getData) {
        console.log(`   Get result: ${getData.toString()}`);
        console.log(`   Match: ${getData.toString() === testData.toString()}`);
    }

    // Test 2: Set and get with JSON serialization
    console.log('\n2. Testing JSON workflow...');
    const user = { name: 'Bob', role: 'admin', id: 42 };
    const jsonData = Buffer.from(JSON.stringify(user));
    await client.set('binary:user', jsonData);

    const retrievedData = await client.get('binary:user');
    if (retrievedData) {
        const parsed = JSON.parse(retrievedData.toString());
        console.log(`   Retrieved user: ${JSON.stringify(parsed)}`);
        console.log(`   Match: ${JSON.stringify(parsed) === JSON.stringify(user)}`);
    }

    // Test 3: Delete
    console.log('\n3. Testing delete...');
    const deleteResult = await client.delete('binary:test');
    console.log(`   Delete result: ${deleteResult}`);

    const getAfterDelete = await client.get('binary:test');
    console.log(`   Get after delete: ${getAfterDelete === null ? 'null (correct)' : 'ERROR: still exists'}`);

    // Test 4: Health check
    console.log('\n4. Testing health check...');
    const health = await client.health();
    if (health) {
        console.log(`   Status: ${health.status}`);
        console.log(`   Cache items: ${health.cache_items}`);
        console.log(`   Goroutines: ${health.goroutines}`);
    }

    // Test 5: Large data
    console.log('\n5. Testing large data (1KB)...');
    const largeData = Buffer.alloc(1024, 'X');
    await client.set('binary:large', largeData);
    const retrievedLarge = await client.get('binary:large');
    console.log(`   Large data size match: ${retrievedLarge?.length === 1024}`);

    console.log('\n✅ All TypeScript Binary client tests passed!');
}

main().catch(console.error);
